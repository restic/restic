package main

import (
	"context"
	"fmt"
	"time"

	"github.com/restic/restic/internal/archiver"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/backup"
	"github.com/restic/restic/internal/ui/termstatus"
	"golang.org/x/sync/errgroup"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func newCopyCommand() *cobra.Command {
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
			term, cancel := setupTermstatus()
			defer cancel()
			return runCopy(cmd.Context(), opts, globalOptions, args, term)
		},
	}

	opts.AddFlags(cmd.Flags())
	return cmd
}

// CopyOptions bundles all options for the copy command.
type CopyOptions struct {
	secondaryRepoOptions
	restic.SnapshotFilter
	startTime time.Time
}

func (opts *CopyOptions) AddFlags(f *pflag.FlagSet) {
	opts.secondaryRepoOptions.AddFlags(f, "destination", "to copy snapshots from")
	initMultiSnapshotFilter(f, &opts.SnapshotFilter, true)
}

func runCopy(ctx context.Context, opts CopyOptions, gopts GlobalOptions, args []string, term *termstatus.Terminal) error {
	opts.startTime = time.Now()
	secondaryGopts, isFromRepo, err := fillSecondaryGlobalOpts(ctx, opts.secondaryRepoOptions, gopts, "destination")
	if err != nil {
		return err
	}
	if isFromRepo {
		// swap global options, if the secondary repo was set via from-repo
		gopts, secondaryGopts = secondaryGopts, gopts
	}

	ctx, srcRepo, unlock, err := openWithReadLock(ctx, gopts, gopts.NoLock)
	if err != nil {
		return err
	}
	defer unlock()

	ctx, dstRepo, unlock, err := openWithAppendLock(ctx, secondaryGopts, false)
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
	bar := newIndexProgress(gopts.Quiet, gopts.JSON)
	if err := srcRepo.LoadIndex(ctx, bar); err != nil {
		return err
	}
	bar = newIndexProgress(gopts.Quiet, gopts.JSON)
	debug.Log("Loading destination index")
	if err := dstRepo.LoadIndex(ctx, bar); err != nil {
		return err
	}

	dstSnapshotByOriginal := make(map[restic.ID][]*restic.Snapshot)
	for sn := range FindFilteredSnapshots(ctx, dstSnapshotLister, dstRepo, &opts.SnapshotFilter, nil) {
		if sn.Original != nil && !sn.Original.IsNull() {
			dstSnapshotByOriginal[*sn.Original] = append(dstSnapshotByOriginal[*sn.Original], sn)
		}
		// also consider identical snapshot copies
		dstSnapshotByOriginal[*sn.ID()] = append(dstSnapshotByOriginal[*sn.ID()], sn)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// remember already processed trees across all snapshots
	visitedTrees := restic.NewIDSet()

	for sn := range FindFilteredSnapshots(ctx, srcSnapshotLister, srcRepo, &opts.SnapshotFilter, args) {
		// check whether the destination has a snapshot with the same persistent ID which has similar snapshot fields
		srcOriginal := *sn.ID()
		if sn.Original != nil {
			srcOriginal = *sn.Original
		}

		if originalSns, ok := dstSnapshotByOriginal[srcOriginal]; ok {
			isCopy := false
			for _, originalSn := range originalSns {
				if similarSnapshots(originalSn, sn) {
					if !gopts.JSON {
						Verboseff("\n%v\n", sn)
						Verboseff("skipping source snapshot %s, was already copied to snapshot %s\n", sn.ID().Str(), originalSn.ID().Str())
					}
					isCopy = true
					break
				}
			}
			if isCopy {
				continue
			}
		}
		if !gopts.JSON {
			Verbosef("\n%v\n", sn)
			Verbosef("  copy started, this may take a while...\n")
		}
		if err := copyTree(ctx, srcRepo, dstRepo, visitedTrees, sn, opts, gopts, term); err != nil {
			return err
		}
		debug.Log("tree copied")
	}
	return ctx.Err()
}

func similarSnapshots(sna *restic.Snapshot, snb *restic.Snapshot) bool {
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

// CopyCounters is a collection of counters, matching the fields of archiver.Summary
// The 'New' / 'Changed' fields are not used, classifying these new/modified
// files and directories in this context does not make much sense.
type CopyCounters struct {
	countTreeBlobs      int
	countDataBlobs      int
	countDirsNew        int
	countDirsChanged    int
	countDirsTotal      int
	countFilesNew       int
	countFilesChanged   int
	countFilesTotal     int
	sizeTreeBlobsInRepo uint
	sizeDataBlobsInRepo uint
	sizeTreeBlobs       uint
	sizeDataBlobs       uint
	sizeTreeBlobsTotal  uint64
	sizeDataBlobsTotal  uint64
}

func copyTree(ctx context.Context, srcRepo restic.Repository, dstRepo restic.Repository,
	visitedTrees restic.IDSet, sn *restic.Snapshot,
	opts CopyOptions, gopts GlobalOptions, term *termstatus.Terminal) error {

	wg, wgCtx := errgroup.WithContext(ctx)

	rootTreeID := *sn.Tree
	treeStream := restic.StreamTrees(wgCtx, wg, srcRepo, restic.IDs{rootTreeID}, func(treeID restic.ID) bool {
		visited := visitedTrees.Has(treeID)
		visitedTrees.Insert(treeID)
		return visited
	}, nil)

	copyBlobs := restic.NewBlobSet()
	packList := restic.NewIDSet()
	copyCounters := CopyCounters{}

	enqueue := func(h restic.BlobHandle) {
		pb := srcRepo.LookupBlob(h.Type, h.ID)
		copyBlobs.Insert(h)
		for _, p := range pb {
			packList.Insert(p.PackID)
		}
		// gather counts and sizes from blobs
		for _, p := range pb {
			if h.Type == restic.TreeBlob {
				copyCounters.countTreeBlobs++
				copyCounters.sizeTreeBlobsInRepo += p.Length
				copyCounters.sizeTreeBlobs += p.UncompressedLength
			} else if h.Type == restic.DataBlob {
				copyCounters.countDataBlobs++
				copyCounters.sizeDataBlobsInRepo += p.Length
				copyCounters.sizeDataBlobs += p.UncompressedLength
			}
		}
	}

	wg.Go(func() error {
		for tree := range treeStream {
			if tree.Error != nil {
				return fmt.Errorf("LoadTree(%v) returned error %v", tree.ID.Str(), tree.Error)
			}

			// Do we already have this tree blob?
			treeHandle := restic.BlobHandle{ID: tree.ID, Type: restic.TreeBlob}
			if _, ok := dstRepo.LookupBlobSize(treeHandle.Type, treeHandle.ID); !ok {
				// copy raw tree bytes to avoid problems if the serialization changes
				enqueue(treeHandle)
			}
			pb := srcRepo.LookupBlob(restic.TreeBlob, tree.ID)
			for _, p := range pb {
				copyCounters.sizeTreeBlobsTotal += uint64(p.UncompressedLength)
			}

			for _, entry := range tree.Nodes {
				// Recursion into directories is handled by StreamTrees
				// Copy the blobs for this file.
				if entry.Type == "dir" {
					copyCounters.countDirsTotal++
				} else if entry.Type == "file" {
					copyCounters.countFilesTotal++
				}
				for _, blobID := range entry.Content {
					h := restic.BlobHandle{Type: restic.DataBlob, ID: blobID}
					if _, ok := dstRepo.LookupBlobSize(h.Type, h.ID); !ok {
						enqueue(h)
					}
					pb := srcRepo.LookupBlob(restic.DataBlob, blobID)
					for _, p := range pb {
						copyCounters.sizeDataBlobsTotal += uint64(p.UncompressedLength)
					}
				}
			}
		}
		return nil
	})
	err := wg.Wait()
	if err != nil {
		return err
	}
	archSummary := setupArchSummary(copyCounters, opts)

	bar := newProgressMax(!gopts.JSON, uint64(len(packList)), "packs copied")
	_, err = repository.Repack(
		ctx,
		srcRepo,
		dstRepo,
		packList,
		copyBlobs,
		bar,
		func(msg string, args ...interface{}) { fmt.Printf(msg+"\n", args...) },
	)
	bar.Done()
	if err != nil {
		return errors.Fatal(err.Error())
	}

	archSummary.BackupEnd = time.Now()
	return createSnapshotSummary(ctx, dstRepo, sn, archSummary, term, gopts)
}

// copy the selected counter / size values to an 'archiver.Summary'
// so it can be shown later
func setupArchSummary(copyCounters CopyCounters, opts CopyOptions) archiver.Summary {

	archSummary := archiver.Summary{
		Files: archiver.ChangeStats{
			New:       uint(copyCounters.countFilesNew),
			Changed:   uint(copyCounters.countFilesChanged),
			Unchanged: uint(copyCounters.countFilesTotal - copyCounters.countFilesNew - copyCounters.countFilesChanged),
		},
		Dirs: archiver.ChangeStats{
			New:       uint(copyCounters.countDirsNew),
			Changed:   uint(copyCounters.countDirsChanged),
			Unchanged: uint(copyCounters.countDirsTotal - copyCounters.countDirsNew - copyCounters.countDirsChanged),
		},
		ItemStats: archiver.ItemStats{
			DataBlobs:      copyCounters.countDataBlobs,
			TreeBlobs:      copyCounters.countTreeBlobs,
			DataSize:       uint64(copyCounters.sizeDataBlobs),
			TreeSize:       uint64(copyCounters.sizeTreeBlobs),
			DataSizeInRepo: uint64(copyCounters.sizeDataBlobsInRepo),
			TreeSizeInRepo: uint64(copyCounters.sizeTreeBlobsInRepo),
		},
		ProcessedBytes: copyCounters.sizeTreeBlobsTotal + copyCounters.sizeDataBlobsTotal,
		BackupStart:    opts.startTime,
	}
	return archSummary
}

func createSnapshotSummary(ctx context.Context, repo restic.Repository, sn *restic.Snapshot,
	archSummary archiver.Summary, term *termstatus.Terminal, gopts GlobalOptions) error {
	// save snapshot
	sn.Parent = nil // Parent does not have relevance in the new repo.
	// Use Original as a persistent snapshot ID
	if sn.Original == nil {
		sn.Original = sn.ID()
	}

	// calculate restic.SnapshotSummary from `archSummary`,
	// copied from internal/archiver/archiver.go
	sn.Summary = &restic.SnapshotSummary{
		BackupStart:         archSummary.BackupStart,
		BackupEnd:           archSummary.BackupEnd,
		FilesNew:            archSummary.Files.New,
		FilesChanged:        archSummary.Files.Changed,
		FilesUnmodified:     archSummary.Files.Unchanged,
		DirsNew:             archSummary.Dirs.New,
		DirsChanged:         archSummary.Dirs.Changed,
		DirsUnmodified:      archSummary.Dirs.Unchanged,
		DataBlobs:           archSummary.ItemStats.DataBlobs,
		TreeBlobs:           archSummary.ItemStats.TreeBlobs,
		DataAdded:           archSummary.ItemStats.DataSize + archSummary.ItemStats.TreeSize,
		DataAddedPacked:     archSummary.ItemStats.DataSizeInRepo + archSummary.ItemStats.TreeSizeInRepo,
		TotalFilesProcessed: archSummary.Files.New + archSummary.Files.Changed + archSummary.Files.Unchanged,
		TotalBytesProcessed: archSummary.ProcessedBytes,
	}

	newID, err := restic.SaveSnapshot(ctx, repo, sn)
	if err != nil {
		return err
	}

	// use `backup.ProgressPrinter` to output `restic copy` details to
	// text output or JSON output.
	// post via progressReporter.Finish
	var progressPrinter backup.ProgressPrinter
	if gopts.JSON {
		progressPrinter = backup.NewJSONProgress(term, gopts.verbosity)
	} else {
		progressPrinter = backup.NewTextProgress(term, gopts.verbosity)
	}
	progressReporter := backup.NewProgress(progressPrinter,
		calculateProgressInterval(!gopts.Quiet, gopts.JSON))
	defer progressReporter.Done()

	progressReporter.Finish(newID, &archSummary, false)
	return nil
}
