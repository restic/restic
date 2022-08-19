package main

import (
	"context"
	"fmt"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/sync/errgroup"

	"github.com/spf13/cobra"
)

var cmdCopy = &cobra.Command{
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
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCopy(copyOptions, globalOptions, args)
	},
}

// CopyOptions bundles all options for the copy command.
type CopyOptions struct {
	secondaryRepoOptions
	Hosts []string
	Tags  restic.TagLists
	Paths []string
}

var copyOptions CopyOptions

func init() {
	cmdRoot.AddCommand(cmdCopy)

	f := cmdCopy.Flags()
	initSecondaryRepoOptions(f, &copyOptions.secondaryRepoOptions, "destination", "to copy snapshots from")
	f.StringArrayVarP(&copyOptions.Hosts, "host", "H", nil, "only consider snapshots for this `host`, when no snapshot ID is given (can be specified multiple times)")
	f.Var(&copyOptions.Tags, "tag", "only consider snapshots which include this `taglist`, when no snapshot ID is given")
	f.StringArrayVar(&copyOptions.Paths, "path", nil, "only consider snapshots which include this (absolute) `path`, when no snapshot ID is given")
}

func runCopy(opts CopyOptions, gopts GlobalOptions, args []string) error {
	secondaryGopts, isFromRepo, err := fillSecondaryGlobalOpts(opts.secondaryRepoOptions, gopts, "destination")
	if err != nil {
		return err
	}
	if isFromRepo {
		// swap global options, if the secondary repo was set via from-repo
		gopts, secondaryGopts = secondaryGopts, gopts
	}

	ctx, cancel := context.WithCancel(gopts.ctx)
	defer cancel()

	srcRepo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	dstRepo, err := OpenRepository(secondaryGopts)
	if err != nil {
		return err
	}

	if !gopts.NoLock {
		srcLock, err := lockRepo(ctx, srcRepo)
		defer unlockRepo(srcLock)
		if err != nil {
			return err
		}
	}

	dstLock, err := lockRepo(ctx, dstRepo)
	defer unlockRepo(dstLock)
	if err != nil {
		return err
	}

	srcSnapshotLister, err := backend.MemorizeList(ctx, srcRepo.Backend(), restic.SnapshotFile)
	if err != nil {
		return err
	}

	dstSnapshotLister, err := backend.MemorizeList(ctx, dstRepo.Backend(), restic.SnapshotFile)
	if err != nil {
		return err
	}

	debug.Log("Loading source index")
	if err := srcRepo.LoadIndex(ctx); err != nil {
		return err
	}

	debug.Log("Loading destination index")
	if err := dstRepo.LoadIndex(ctx); err != nil {
		return err
	}

	dstSnapshotByOriginal := make(map[restic.ID][]*restic.Snapshot)
	for sn := range FindFilteredSnapshots(ctx, dstSnapshotLister, dstRepo, opts.Hosts, opts.Tags, opts.Paths, nil) {
		if sn.Original != nil && !sn.Original.IsNull() {
			dstSnapshotByOriginal[*sn.Original] = append(dstSnapshotByOriginal[*sn.Original], sn)
		}
		// also consider identical snapshot copies
		dstSnapshotByOriginal[*sn.ID()] = append(dstSnapshotByOriginal[*sn.ID()], sn)
	}

	// remember already processed trees across all snapshots
	visitedTrees := restic.NewIDSet()

	for sn := range FindFilteredSnapshots(ctx, srcSnapshotLister, srcRepo, opts.Hosts, opts.Tags, opts.Paths, args) {
		Verbosef("\nsnapshot %s of %v at %s)\n", sn.ID().Str(), sn.Paths, sn.Time)

		// check whether the destination has a snapshot with the same persistent ID which has similar snapshot fields
		srcOriginal := *sn.ID()
		if sn.Original != nil {
			srcOriginal = *sn.Original
		}
		if originalSns, ok := dstSnapshotByOriginal[srcOriginal]; ok {
			isCopy := false
			for _, originalSn := range originalSns {
				if similarSnapshots(originalSn, sn) {
					Verbosef("skipping source snapshot %s, was already copied to snapshot %s\n", sn.ID().Str(), originalSn.ID().Str())
					isCopy = true
					break
				}
			}
			if isCopy {
				continue
			}
		}
		Verbosef("  copy started, this may take a while...\n")
		if err := copyTree(ctx, srcRepo, dstRepo, visitedTrees, *sn.Tree, gopts.Quiet); err != nil {
			return err
		}
		debug.Log("tree copied")

		// save snapshot
		sn.Parent = nil // Parent does not have relevance in the new repo.
		// Use Original as a persistent snapshot ID
		if sn.Original == nil {
			sn.Original = sn.ID()
		}
		newID, err := restic.SaveSnapshot(ctx, dstRepo, sn)
		if err != nil {
			return err
		}
		Verbosef("snapshot %s saved\n", newID.Str())
	}
	return nil
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

func copyTree(ctx context.Context, srcRepo restic.Repository, dstRepo restic.Repository,
	visitedTrees restic.IDSet, rootTreeID restic.ID, quiet bool) error {

	wg, wgCtx := errgroup.WithContext(ctx)

	treeStream := restic.StreamTrees(wgCtx, wg, srcRepo, restic.IDs{rootTreeID}, func(treeID restic.ID) bool {
		visited := visitedTrees.Has(treeID)
		visitedTrees.Insert(treeID)
		return visited
	}, nil)

	copyBlobs := restic.NewBlobSet()
	packList := restic.NewIDSet()

	enqueue := func(h restic.BlobHandle) {
		pb := srcRepo.Index().Lookup(h)
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
			if !dstRepo.Index().Has(treeHandle) {
				// copy raw tree bytes to avoid problems if the serialization changes
				enqueue(treeHandle)
			}

			for _, entry := range tree.Nodes {
				// Recursion into directories is handled by StreamTrees
				// Copy the blobs for this file.
				for _, blobID := range entry.Content {
					h := restic.BlobHandle{Type: restic.DataBlob, ID: blobID}
					if !dstRepo.Index().Has(h) {
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

	bar := newProgressMax(!quiet, uint64(len(packList)), "packs copied")
	_, err = repository.Repack(ctx, srcRepo, dstRepo, packList, copyBlobs, bar)
	bar.Done()
	return err
}
