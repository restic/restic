package main

import (
	"context"
	"fmt"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"

	"github.com/spf13/cobra"
)

var cmdCopy = &cobra.Command{
	Use:   "copy [flags] [snapshotID ...]",
	Short: "Copy snapshots from one repository to another",
	Long: `
The "copy" command copies one or more snapshots from one repository to another
repository. Note that this will have to read (download) and write (upload) the
entire snapshot(s) due to the different encryption keys on the source and
destination, and that transferred files are not re-chunked, which may break
their deduplication. This can be mitigated by the "--copy-chunker-params"
option when initializing a new destination repository using the "init" command.
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
	initSecondaryRepoOptions(f, &copyOptions.secondaryRepoOptions, "destination", "to copy snapshots to")
	f.StringArrayVarP(&copyOptions.Hosts, "host", "H", nil, "only consider snapshots for this `host`, when no snapshot ID is given (can be specified multiple times)")
	f.Var(&copyOptions.Tags, "tag", "only consider snapshots which include this `taglist`, when no snapshot ID is given")
	f.StringArrayVar(&copyOptions.Paths, "path", nil, "only consider snapshots which include this (absolute) `path`, when no snapshot ID is given")
}

func runCopy(opts CopyOptions, gopts GlobalOptions, args []string) error {
	dstGopts, err := fillSecondaryGlobalOpts(opts.secondaryRepoOptions, gopts, "destination")
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(gopts.ctx)
	defer cancel()

	srcRepo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	dstRepo, err := OpenRepository(dstGopts)
	if err != nil {
		return err
	}

	srcLock, err := lockRepo(ctx, srcRepo)
	defer unlockRepo(srcLock)
	if err != nil {
		return err
	}

	dstLock, err := lockRepo(ctx, dstRepo)
	defer unlockRepo(dstLock)
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
	for sn := range FindFilteredSnapshots(ctx, dstRepo, opts.Hosts, opts.Tags, opts.Paths, nil) {
		if sn.Original != nil && !sn.Original.IsNull() {
			dstSnapshotByOriginal[*sn.Original] = append(dstSnapshotByOriginal[*sn.Original], sn)
		}
		// also consider identical snapshot copies
		dstSnapshotByOriginal[*sn.ID()] = append(dstSnapshotByOriginal[*sn.ID()], sn)
	}

	cloner := &treeCloner{
		srcRepo:      srcRepo,
		dstRepo:      dstRepo,
		visitedTrees: restic.NewIDSet(),
		buf:          nil,
	}

	for sn := range FindFilteredSnapshots(ctx, srcRepo, opts.Hosts, opts.Tags, opts.Paths, args) {
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

		if err := cloner.copyTree(ctx, *sn.Tree); err != nil {
			return err
		}
		debug.Log("tree copied")

		if err = dstRepo.Flush(ctx); err != nil {
			return err
		}
		debug.Log("flushed packs and saved index")

		// save snapshot
		sn.Parent = nil // Parent does not have relevance in the new repo.
		// Use Original as a persistent snapshot ID
		if sn.Original == nil {
			sn.Original = sn.ID()
		}
		newID, err := dstRepo.SaveJSONUnpacked(ctx, restic.SnapshotFile, sn)
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

type treeCloner struct {
	srcRepo      restic.Repository
	dstRepo      restic.Repository
	visitedTrees restic.IDSet
	buf          []byte
}

func (t *treeCloner) copyTree(ctx context.Context, treeID restic.ID) error {
	// We have already processed this tree
	if t.visitedTrees.Has(treeID) {
		return nil
	}

	tree, err := t.srcRepo.LoadTree(ctx, treeID)
	if err != nil {
		return fmt.Errorf("LoadTree(%v) returned error %v", treeID.Str(), err)
	}
	t.visitedTrees.Insert(treeID)

	// Do we already have this tree blob?
	if !t.dstRepo.Index().Has(restic.BlobHandle{ID: treeID, Type: restic.TreeBlob}) {
		newTreeID, err := t.dstRepo.SaveTree(ctx, tree)
		if err != nil {
			return fmt.Errorf("SaveTree(%v) returned error %v", treeID.Str(), err)
		}
		// Assurance only.
		if newTreeID != treeID {
			return fmt.Errorf("SaveTree(%v) returned unexpected id %s", treeID.Str(), newTreeID.Str())
		}
	}

	// TODO: parellize this stuff, likely only needed inside a tree.

	for _, entry := range tree.Nodes {
		// If it is a directory, recurse
		if entry.Type == "dir" && entry.Subtree != nil {
			if err := t.copyTree(ctx, *entry.Subtree); err != nil {
				return err
			}
		}
		// Copy the blobs for this file.
		for _, blobID := range entry.Content {
			// Do we already have this data blob?
			if t.dstRepo.Index().Has(restic.BlobHandle{ID: blobID, Type: restic.DataBlob}) {
				continue
			}
			debug.Log("Copying blob %s\n", blobID.Str())
			t.buf, err = t.srcRepo.LoadBlob(ctx, restic.DataBlob, blobID, t.buf)
			if err != nil {
				return fmt.Errorf("LoadBlob(%v) returned error %v", blobID, err)
			}

			_, _, err = t.dstRepo.SaveBlob(ctx, restic.DataBlob, t.buf, blobID, false)
			if err != nil {
				return fmt.Errorf("SaveBlob(%v) returned error %v", blobID, err)
			}
		}
	}

	return nil
}
