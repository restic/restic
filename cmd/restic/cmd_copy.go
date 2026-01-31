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
	"github.com/restic/restic/internal/global"
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
}

func (opts *CopyOptions) AddFlags(f *pflag.FlagSet) {
	opts.SecondaryRepoOptions.AddFlags(f, "destination", "to copy snapshots from")
	initMultiSnapshotFilter(f, &opts.SnapshotFilter, true)
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

	if err := copyTreeBatched(ctx, srcRepo, dstRepo, selectedSnapshots, printer); err != nil {
		return err
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
