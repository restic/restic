package main

import (
	"context"
	"fmt"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"
	"golang.org/x/sync/errgroup"

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
	batch bool
}

func (opts *CopyOptions) AddFlags(f *pflag.FlagSet) {
	f.BoolVar(&opts.batch, "batch", false, "batch all snapshots to be copied into one step to optimize use of packfiles")
	opts.SecondaryRepoOptions.AddFlags(f, "destination", "to copy snapshots from")
	initMultiSnapshotFilter(f, &opts.SnapshotFilter, true)
}

// collectAllSnapshots: select all snapshot trees to be copied
func collectAllSnapshots(ctx context.Context, opts CopyOptions,
	srcSnapshotLister restic.Lister, srcRepo restic.Repository,
	dstSnapshotByOriginal map[restic.ID][]*data.Snapshot, args []string, printer progress.Printer,
) (selectedSnapshots []*data.Snapshot) {

	selectedSnapshots = make([]*data.Snapshot, 0, 10)
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
		selectedSnapshots = append(selectedSnapshots, sn)
	}

	return selectedSnapshots
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

	if err := copyTreeBatched(ctx, srcRepo, dstRepo, selectedSnapshots, opts, printer); err != nil {
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

// copyTreeBatched: copy multiple snapshot trees in one go, using calls to
// repository.RepackInner() for all selected snapshot trees and thereby packing the packfiles optimally.
// Usually each snapshot creates at least one tree packfile and one data packfile.
func copyTreeBatched(ctx context.Context, srcRepo restic.Repository, dstRepo restic.Repository,
	selectedSnapshots []*data.Snapshot, opts CopyOptions, printer progress.Printer) error {

	// remember already processed trees across all snapshots
	visitedTrees := restic.NewIDSet()

	// dependent on opts.batch the pack uploader is started either for
	// each snapshot to be copied or once for all snapshots
	if opts.batch {
		// call WithBlobUploader() once and then loop over all selectedSnapshots
		err := dstRepo.WithBlobUploader(ctx, func(ctx context.Context, uploader restic.BlobSaver) error {
			for _, sn := range selectedSnapshots {
				printer.P("\n%v", sn)
				printer.P("  copy started, this may take a while...")
				err := copyTree(ctx, srcRepo, dstRepo, visitedTrees, *sn.Tree, printer, uploader)
				if err != nil {
					return err
				}
				debug.Log("tree copied")
			}

			// save all the snapshots
			for _, sn := range selectedSnapshots {
				err := copySaveSnapshot(ctx, sn, dstRepo, printer)
				if err != nil {
					return err
				}
			}
			return nil
		})

		return err
	}

	// no batch option, loop over selectedSnapshots and call WithBlobUploader()
	// inside the loop
	for _, sn := range selectedSnapshots {
		printer.P("\n%v", sn)
		printer.P("  copy started, this may take a while...")
		err := dstRepo.WithBlobUploader(ctx, func(ctx context.Context, uploader restic.BlobSaver) error {
			if err := copyTree(ctx, srcRepo, dstRepo, visitedTrees, *sn.Tree, printer, uploader); err != nil {
				return err
			}
			debug.Log("tree copied")
			return nil
		})
		if err != nil {
			return err
		}

		err = copySaveSnapshot(ctx, sn, dstRepo, printer)
		if err != nil {
			return err
		}
	}

	return nil
}

func copyTree(ctx context.Context, srcRepo restic.Repository, dstRepo restic.Repository,
	visitedTrees restic.IDSet, rootTreeID restic.ID, printer progress.Printer, uploader restic.BlobSaver) error {

	wg, wgCtx := errgroup.WithContext(ctx)

	treeStream := data.StreamTrees(wgCtx, wg, srcRepo, restic.IDs{rootTreeID}, func(treeID restic.ID) bool {
		visited := visitedTrees.Has(treeID)
		visitedTrees.Insert(treeID)
		return visited
	}, nil)

	copyBlobs := restic.NewBlobSet()
	packList := restic.NewIDSet()

	enqueue := func(h restic.BlobHandle) {
		pb := srcRepo.LookupBlob(h.Type, h.ID)
		copyBlobs.Insert(h)
		for _, p := range pb {
			packList.Insert(p.PackID)
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

			for _, entry := range tree.Nodes {
				// Recursion into directories is handled by StreamTrees
				// Copy the blobs for this file.
				for _, blobID := range entry.Content {
					h := restic.BlobHandle{Type: restic.DataBlob, ID: blobID}
					if _, ok := dstRepo.LookupBlobSize(h.Type, h.ID); !ok {
						enqueue(h)
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

	copyStats(srcRepo, copyBlobs, packList, printer)
	bar := printer.NewCounter("packs copied")
	err = repository.CopyBlobs(ctx, srcRepo, dstRepo, uploader, packList, copyBlobs, bar, printer.P)
	if err != nil {
		return errors.Fatalf("%s", err)
	}
	return nil
}

// copyStats: print statistics for the blobs to be copied
func copyStats(srcRepo restic.Repository, copyBlobs restic.BlobSet, packList restic.IDSet, printer progress.Printer) {

	// count and size
	countBlobs := 0
	sizeBlobs := uint64(0)
	for blob := range copyBlobs {
		for _, blob := range srcRepo.LookupBlob(blob.Type, blob.ID) {
			countBlobs++
			sizeBlobs += uint64(blob.Length)
			break
		}
	}

	printer.V("  copy %d blobs with disk size %s in %d packfiles\n",
		countBlobs, ui.FormatBytes(uint64(sizeBlobs)), len(packList))
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
	printer.P("snapshot %s saved", newID.Str())
	return nil
}
