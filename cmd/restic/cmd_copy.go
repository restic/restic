package main

import (
	"context"
	"fmt"
	"iter"
	"sync"
	"time"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/feature"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/rechunker"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func newCopyCommand(globalOptions *global.Options) *cobra.Command {
	var opts CopyOptions
	cmd := &cobra.Command{
		Use:   "copy [flags] [snapshotID ...]",
		Short: "Copy snapshots from one repository to another",
		Long: `
The "copy" command copies one or more snapshots from one repository to another.

NOTE: This process will have to both download (read) and upload (write) the
entire snapshot(s) due to the different encryption keys used in the source and
destination repositories. This /may incur higher bandwidth usage and costs/ than
expected during normal backup runs.

NOTE: The copying process does not re-chunk files, which may break deduplication
between the files copied and files already stored in the destination repository.
This means that copied files, which existed in both the source and destination
repository, /may occupy up to twice their space/ in the destination repository.
This can be mitigated by the "--copy-chunker-params" option when initializing a
new destination repository using the "init" command.

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
Exit status is 10 if the repository does not exist.
Exit status is 11 if the repository is already locked.
Exit status is 12 if the password is incorrect.
`,
		GroupID:           cmdGroupDefault,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			finalizeSnapshotFilter(&opts.SnapshotFilter)
			return runCopy(cmd.Context(), opts, *globalOptions, args, globalOptions.Term)
		},
	}

	opts.AddFlags(cmd.Flags())
	return cmd
}

// CopyOptions bundles all options for the copy command.
type CopyOptions struct {
	global.SecondaryRepoOptions
	data.SnapshotFilter
	RechunkCopyOptions
}

func (opts *CopyOptions) AddFlags(f *pflag.FlagSet) {
	opts.SecondaryRepoOptions.AddFlags(f, "destination", "to copy snapshots from")
	initMultiSnapshotFilter(f, &opts.SnapshotFilter, true)
	opts.RechunkCopyOptions.AddFlags(f)
}

type RechunkCopyOptions struct {
	Rechunk           bool
	ForceRechunk      bool
	AddTags           data.TagLists
	CacheSize         int
	isIntegrationTest bool // skip check for RESTIC_FEATURES=rechunk-copy during integration test
}

func (opts *RechunkCopyOptions) AddFlags(f *pflag.FlagSet) {
	f.BoolVar(&opts.Rechunk, "rechunk", false, "rechunk files when copying")
	f.BoolVar(&opts.ForceRechunk, "force", false, "force rechunk even when src and dst repo have same chunker polynomials; to be used with --rechunk")
	f.IntVar(&opts.CacheSize, "cache-size", 4096, "for rechunk copy, specify in-memory blob cache size in MiBs (0 to disable cache). Used with --rechunk")
	f.Var(&opts.AddTags, "add-tag", "add `tags` for the copied snapshots in the format `tag[,tag,...]` (can be specified multiple times). Used with --rechunk")
}

// collectAllSnapshots: select all snapshot trees to be copied
func collectAllSnapshots(ctx context.Context, opts CopyOptions,
	srcSnapshotLister restic.Lister, srcRepo restic.Repository,
	dstSnapshotByOriginal map[restic.ID][]*data.Snapshot, args []string, printer progress.Printer,
) iter.Seq[*data.Snapshot] {
	return func(yield func(*data.Snapshot) bool) {
		for sn := range FindFilteredSnapshots(ctx, srcSnapshotLister, srcRepo, &opts.SnapshotFilter, args, printer) {
			// check whether the destination has a snapshot with the same persistent ID which has similar snapshot fields
			srcOriginal := *sn.ID()
			if sn.Original != nil {
				srcOriginal = *sn.Original
			}
			if originalSns, ok := dstSnapshotByOriginal[srcOriginal]; ok {
				isCopy := false
				for _, originalSn := range originalSns {
					if similarSnapshots(originalSn, sn) {
						printer.V("\n%v", sn)
						printer.V("skipping source snapshot %s, was already copied to snapshot %s", sn.ID().Str(), originalSn.ID().Str())
						isCopy = true
						break
					}
				}
				if isCopy {
					continue
				}
			}
			if !yield(sn) {
				return
			}
		}
	}
}

func runCopy(ctx context.Context, opts CopyOptions, gopts global.Options, args []string, term ui.Terminal) error {
	// Rechunk-copy guardrails
	if opts.Rechunk {
		debug.Log("Rechunk option enabled")
		if !feature.Flag.Enabled(feature.RechunkCopy) && !opts.isIntegrationTest {
			return errors.Fatal("rechunk-copy feature flag is not set. Currently, rechunk-copy is alpha feature (disabled by default).")
		}
		if opts.CacheSize != 0 && opts.CacheSize < 100 {
			return errors.Fatal("blob cache size must be at least 100 MiB")
		}
	}

	printer := ui.NewProgressPrinter(false, gopts.Verbosity, term)
	secondaryGopts, isFromRepo, err := opts.SecondaryRepoOptions.FillGlobalOpts(ctx, gopts, "destination")
	if err != nil {
		return err
	}
	if isFromRepo {
		// swap global options, if the secondary repo was set via from-repo
		gopts, secondaryGopts = secondaryGopts, gopts
	}

	ctx, srcRepo, unlock, err := openWithReadLock(ctx, gopts, gopts.NoLock, printer)
	if err != nil {
		return err
	}
	defer unlock()

	ctx, dstRepo, unlock, err := openWithAppendLock(ctx, secondaryGopts, false, printer)
	if err != nil {
		return err
	}
	defer unlock()

	// if rechunk is enabled, ensure srcRepo and dstRepo have different ChunkerPolynomials
	if opts.Rechunk && !opts.ForceRechunk && srcRepo.Config().ChunkerPolynomial == dstRepo.Config().ChunkerPolynomial {
		return errors.Fatal("source repo and destination repo have same chunker polynomials; run without `--rechunk`, or set `--force` flag to proceed with rechunk anyway")
	}

	srcSnapshotLister, err := restic.MemorizeList(ctx, srcRepo, restic.SnapshotFile)
	if err != nil {
		return err
	}

	dstSnapshotLister, err := restic.MemorizeList(ctx, dstRepo, restic.SnapshotFile)
	if err != nil {
		return err
	}

	debug.Log("Loading source index")
	if err := srcRepo.LoadIndex(ctx, printer); err != nil {
		return err
	}
	debug.Log("Loading destination index")
	if err := dstRepo.LoadIndex(ctx, printer); err != nil {
		return err
	}

	dstSnapshotByOriginal := make(map[restic.ID][]*data.Snapshot)
	for sn := range FindFilteredSnapshots(ctx, dstSnapshotLister, dstRepo, &opts.SnapshotFilter, nil, printer) {
		if sn.Original != nil && !sn.Original.IsNull() {
			dstSnapshotByOriginal[*sn.Original] = append(dstSnapshotByOriginal[*sn.Original], sn)
		}
		// also consider identical snapshot copies
		dstSnapshotByOriginal[*sn.ID()] = append(dstSnapshotByOriginal[*sn.ID()], sn)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	selectedSnapshots := collectAllSnapshots(ctx, opts, srcSnapshotLister, srcRepo, dstSnapshotByOriginal, args, printer)

	if !opts.Rechunk {
		if err := copyTreeBatched(ctx, srcRepo, dstRepo, selectedSnapshots, printer); err != nil {
			return err
		}
	} else {
		rechnker := rechunker.NewRechunker(rechunker.Config{
			CacheSize: opts.CacheSize * (1 << 20),
			Pol:       dstRepo.Config().ChunkerPolynomial,
		})
		progress := rechunker.NewProgress(
			term,
			printer,
			ui.CalculateProgressInterval(!gopts.Quiet, gopts.JSON, term.CanUpdateStatus()),
		)
		if err := rechunkCopy(ctx, srcRepo, dstRepo, selectedSnapshots, rechnker, printer, progress, opts.AddTags.Flatten()); err != nil {
			return err
		}
	}

	return ctx.Err()
}

func similarSnapshots(sna *data.Snapshot, snb *data.Snapshot) bool {
	// everything except Parent and Original must match
	if !sna.Time.Equal(snb.Time) || !sna.Tree.Equal(*snb.Tree) || sna.Hostname != snb.Hostname ||
		sna.Username != snb.Username || sna.UID != snb.UID || sna.GID != snb.GID ||
		len(sna.Paths) != len(snb.Paths) || len(sna.Excludes) != len(snb.Excludes) ||
		len(sna.Tags) != len(snb.Tags) {
		return false
	}
	if !sna.HasPaths(snb.Paths) || !sna.HasTags(snb.Tags) {
		return false
	}
	for i, a := range sna.Excludes {
		if a != snb.Excludes[i] {
			return false
		}
	}
	return true
}

// copyTreeBatched copies multiple snapshots in one go. Snapshots are written after
// data equivalent to at least 10 packfiles was written.
func copyTreeBatched(ctx context.Context, srcRepo restic.Repository, dstRepo restic.Repository,
	selectedSnapshots iter.Seq[*data.Snapshot], printer progress.Printer) error {

	// remember already processed trees across all snapshots
	visitedTrees := srcRepo.NewAssociatedBlobSet()

	targetSize := uint64(dstRepo.PackSize()) * 100
	minDuration := 1 * time.Minute

	// use pull-based iterator to allow iteration in multiple steps
	next, stop := iter.Pull(selectedSnapshots)
	defer stop()

	for {
		var batch []*data.Snapshot
		batchSize := uint64(0)
		startTime := time.Now()

		// call WithBlobUploader() once and then loop over all selectedSnapshots
		err := dstRepo.WithBlobUploader(ctx, func(ctx context.Context, uploader restic.BlobSaverWithAsync) error {
			for batchSize < targetSize || time.Since(startTime) < minDuration {
				sn, ok := next()
				if !ok {
					break
				}

				batch = append(batch, sn)

				printer.P("\n%v", sn)
				printer.P("  copy started, this may take a while...")
				sizeBlobs, err := copyTree(ctx, srcRepo, dstRepo, visitedTrees, *sn.Tree, printer, uploader)
				if err != nil {
					return err
				}
				debug.Log("tree copied")
				batchSize += sizeBlobs
			}

			return nil
		})
		if err != nil {
			return err
		}

		// if no snapshots were processed in this batch, we're done
		if len(batch) == 0 {
			break
		}

		// add a newline to separate saved snapshot messages from the other messages
		if len(batch) > 1 {
			printer.P("")
		}
		// save all the snapshots
		for _, sn := range batch {
			err := copySaveSnapshot(ctx, sn, dstRepo, printer)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func copyTree(ctx context.Context, srcRepo restic.Repository, dstRepo restic.Repository,
	visitedTrees restic.AssociatedBlobSet, rootTreeID restic.ID, printer progress.Printer, uploader restic.BlobSaverWithAsync) (uint64, error) {

	copyBlobs := srcRepo.NewAssociatedBlobSet()
	packList := restic.NewIDSet()
	var lock sync.Mutex

	enqueue := func(h restic.BlobHandle) {
		lock.Lock()
		defer lock.Unlock()
		if _, ok := dstRepo.LookupBlobSize(h.Type, h.ID); !ok {
			pb := srcRepo.LookupBlob(h.Type, h.ID)
			copyBlobs.Insert(h)
			for _, p := range pb {
				packList.Insert(p.PackID)
			}
		}
	}

	err := data.StreamTrees(ctx, srcRepo, restic.IDs{rootTreeID}, nil, func(treeID restic.ID) bool {
		handle := restic.BlobHandle{ID: treeID, Type: restic.TreeBlob}
		visited := visitedTrees.Has(handle)
		visitedTrees.Insert(handle)
		return visited
	}, func(treeID restic.ID, err error, nodes data.TreeNodeIterator) error {
		if err != nil {
			return fmt.Errorf("LoadTree(%v) returned error %v", treeID.Str(), err)
		}

		// copy raw tree bytes to avoid problems if the serialization changes
		enqueue(restic.BlobHandle{ID: treeID, Type: restic.TreeBlob})

		for item := range nodes {
			if item.Error != nil {
				return item.Error
			}
			// Recursion into directories is handled by StreamTrees
			// Copy the blobs for this file.
			for _, blobID := range item.Node.Content {
				enqueue(restic.BlobHandle{Type: restic.DataBlob, ID: blobID})
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	sizeBlobs := copyStats(srcRepo, copyBlobs, packList, printer)
	bar := printer.NewCounter("packs copied")
	err = repository.CopyBlobs(ctx, srcRepo, dstRepo, uploader, packList, copyBlobs, bar, printer.P)
	if err != nil {
		return 0, errors.Fatalf("%s", err)
	}
	return sizeBlobs, nil
}

// copyStats: print statistics for the blobs to be copied
func copyStats(srcRepo restic.Repository, copyBlobs restic.AssociatedBlobSet, packList restic.IDSet, printer progress.Printer) uint64 {
	// count and size
	countBlobs := 0
	sizeBlobs := uint64(0)
	for blob := range copyBlobs.Keys() {
		for _, blob := range srcRepo.LookupBlob(blob.Type, blob.ID) {
			countBlobs++
			sizeBlobs += uint64(blob.Length)
			break
		}
	}

	printer.V("  copy %d blobs with disk size %s in %d packfiles\n",
		countBlobs, ui.FormatBytes(uint64(sizeBlobs)), len(packList))
	return sizeBlobs
}

func copySaveSnapshot(ctx context.Context, sn *data.Snapshot, dstRepo restic.Repository, printer progress.Printer) error {
	sn.Parent = nil // Parent does not have relevance in the new repo.
	// Use Original as a persistent snapshot ID
	if sn.Original == nil {
		sn.Original = sn.ID()
	}
	newID, err := data.SaveSnapshot(ctx, dstRepo, sn)
	if err != nil {
		return err
	}
	printer.P("snapshot %s saved, copied from source snapshot %s", newID.Str(), sn.ID().Str())
	return nil
}

func rechunkCopy(ctx context.Context, srcRepo, dstRepo restic.Repository, selectedSnapshots iter.Seq[*data.Snapshot],
	rechnker *rechunker.Rechunker, printer progress.Printer, progress *rechunker.Progress, tags data.TagList) error {
	printer.V("Gathering snapshots...")
	var snapshots []*data.Snapshot
	var rootTrees restic.IDs
	debug.Log("Gathering root trees from selectedSnapshots()")
	selectedSnapshots(func(sn *data.Snapshot) bool {
		snapshots = append(snapshots, sn)
		rootTrees = append(rootTrees, *sn.Tree)
		return true
	})

	printer.V("Scanning files to process... ")
	debug.Log("Running Plan()")
	err := rechnker.Plan(ctx, srcRepo, rootTrees)
	if err != nil {
		return err
	}

	printer.V("\n[Pre-run Summary]")
	// num_snapshots, num_distinct_files, total_size, num_packs,
	printer.V("Number of snapshots: %v", len(rootTrees))
	printer.V("Number of distinct files to process: %v", rechnker.NumFiles())
	printer.V("  - Total size (including duplicate blobs): %v", ui.FormatBytes(rechnker.TotalSize()))
	printer.V("Number of packs to download: %v\n\n", rechnker.PackCount())

	debug.Log("Running RechunkData()")
	progress.Start(rechnker.NumFiles(), rechnker.TotalSize())
	err = rechnker.Rechunk(ctx, srcRepo, dstRepo, progress)
	if err != nil {
		return err
	}
	progress.Done()

	printer.V("\nRewriting trees...")
	err = dstRepo.WithBlobUploader(ctx, func(ctx context.Context, uploader restic.BlobSaverWithAsync) error {
		for _, tree := range rootTrees {
			debug.Log("Running RewriteTree() for tree ID %v", tree.Str())
			_, err := rechnker.RewriteTree(ctx, srcRepo, uploader, tree)
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return err
	}
	printer.V("Rewriting done.\n\n")

	printer.V("Writing snapshots")
	for _, sn := range snapshots {
		newTreeID, err := rechnker.GetRewrittenTree(*sn.Tree)
		if err != nil {
			return err
		}
		sn.Tree = &newTreeID
		sn.AddTags(tags)
		if err = copySaveSnapshot(ctx, sn, dstRepo, printer); err != nil {
			return err
		}
	}

	printer.P("Additional data stored to the repository: %v", ui.FormatBytes(rechnker.TotalAddedToDstRepo()))

	return nil
}
