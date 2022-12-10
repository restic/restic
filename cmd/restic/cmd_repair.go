package main

import (
	"context"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/sync/errgroup"

	"github.com/spf13/cobra"
)

var cmdRepair = &cobra.Command{
	Use:   "repair [flags] [snapshot ID] [...]",
	Short: "Repair snapshots",
	Long: `
The "repair" command allows to repair broken snapshots.
It scans the given snapshots and generates new ones where
damaged tress and file contents are removed.
If the broken snapshots are deleted, a prune run will
be able to refit the repository.

The command depends on a good state of the index, so if
there are inaccurancies in the index, make sure to run
"rebuild-index" before!


WARNING:
========
Repairing and deleting broken snapshots causes data loss!
It will remove broken dirs and modify broken files in
the modified snapshots.

If the contents of directories and files are still available,
the better option is to redo a backup which in that case is 
able to "heal" already present snapshots.
Only use this command if you need to recover an old and
broken snapshot!

EXIT STATUS
===========

Exit status is 0 if the command was successful, and non-zero if there was any error.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRepair(cmd.Context(), globalOptions, repairOptions, args)
	},
}

// RepairOptions collects all options for the repair command.
type RepairOptions struct {
	restic.SnapshotFilter

	AddTag          string
	Append          string
	DryRun          bool
	DeleteSnapshots bool
}

var repairOptions RepairOptions

func init() {
	cmdRoot.AddCommand(cmdRepair)
	flags := cmdRepair.Flags()

	initMultiSnapshotFilter(flags, &repairOptions.SnapshotFilter, true)

	flags.StringVar(&repairOptions.AddTag, "add-tag", "repaired", "tag to add to repaired snapshots")
	flags.StringVar(&repairOptions.Append, "append", ".repaired", "string to append to repaired dirs/files; remove files if empty or impossible to repair")
	flags.BoolVarP(&repairOptions.DryRun, "dry-run", "n", true, "don't do anything, only show what would be done")
	flags.BoolVar(&repairOptions.DeleteSnapshots, "delete-snapshots", false, "delete original snapshots")
}

func runRepair(ctx context.Context, gopts GlobalOptions, opts RepairOptions, args []string) error {
	switch {
	case opts.DryRun:
		Printf("\n note: --dry-run is set\n-> repair will only show what it would do.\n\n")
	case opts.DeleteSnapshots:
		Printf("\n note: --dry-run is not set and --delete-snapshots is set\n-> this may result in data loss!\n\n")
	}

	repo, err := OpenRepository(ctx, globalOptions)
	if err != nil {
		return err
	}

	lock, ctx, err := lockRepoExclusive(ctx, repo, gopts.RetryLock, gopts.JSON)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	if err := repo.LoadIndex(ctx); err != nil {
		return err
	}

	// get snapshots to check & repair
	var snapshots []*restic.Snapshot
	for sn := range FindFilteredSnapshots(ctx, repo.Backend(), repo, &opts.SnapshotFilter, args) {
		snapshots = append(snapshots, sn)
	}

	return repairSnapshots(ctx, opts, repo, snapshots)
}

func repairSnapshots(ctx context.Context, opts RepairOptions, repo restic.Repository, snapshots []*restic.Snapshot) error {
	replaces := make(idMap)
	seen := restic.NewIDSet()
	deleteSn := restic.NewIDSet()

	Verbosef("check and repair %d snapshots\n", len(snapshots))
	bar := newProgressMax(!globalOptions.Quiet, uint64(len(snapshots)), "snapshots")
	wg, ctx := errgroup.WithContext(ctx)
	repo.StartPackUploader(ctx, wg)
	wg.Go(func() error {
		for _, sn := range snapshots {
			debug.Log("process snapshot %v", sn.ID())
			Printf("%v:\n", sn)
			newID, changed, lErr, err := repairTree(ctx, opts, repo, "/", sn.Tree, replaces, seen)
			switch {
			case err != nil:
				return err
			case lErr:
				Printf("the root tree is damaged -> delete snapshot.\n")
				deleteSn.Insert(*sn.ID())
			case changed:
				err = changeSnapshot(ctx, opts, repo, sn, newID)
				if err != nil {
					return err
				}
				deleteSn.Insert(*sn.ID())
			default:
				Printf("is ok.\n")
			}
			debug.Log("processed snapshot %v", sn.ID())
			bar.Add(1)
		}
		bar.Done()
		return repo.Flush(ctx)
	})

	err := wg.Wait()
	if err != nil {
		return err
	}

	if len(deleteSn) > 0 && opts.DeleteSnapshots {
		Verbosef("delete %d snapshots...\n", len(deleteSn))
		if !opts.DryRun {
			DeleteFiles(ctx, globalOptions, repo, deleteSn, restic.SnapshotFile)
		}
	}
	return nil
}

// changeSnapshot creates a modified snapshot:
// - set the tree to newID
// - add the rag opts.AddTag
// - preserve original ID
// if opts.DryRun is set, it doesn't change anything but only
func changeSnapshot(ctx context.Context, opts RepairOptions, repo restic.Repository, sn *restic.Snapshot, newID *restic.ID) error {
	sn.AddTags([]string{opts.AddTag})
	// Retain the original snapshot id over all tag changes.
	if sn.Original == nil {
		sn.Original = sn.ID()
	}
	sn.Tree = newID
	if !opts.DryRun {
		newID, err := restic.SaveSnapshot(ctx, repo, sn)
		if err != nil {
			return err
		}
		Printf("snapshot repaired -> %v created.\n", newID.Str())
	} else {
		Printf("would have repaired snapshot %v.\n", sn.ID().Str())
	}
	return nil
}

type idMap map[restic.ID]restic.ID

// repairTree checks and repairs a tree and all its subtrees
// Three error cases are checked:
// - tree is a nil tree (-> will be replaced by an empty tree)
// - trees which cannot be loaded (-> the tree contents will be removed)
// - files whose contents are not fully available  (-> file will be modified)
// In case of an error, the changes made depends on:
// - opts.Append: string to append to "repared" names; if empty files will not repaired but deleted
// - opts.DryRun: if set to true, only print out what to but don't change anything
// Returns:
// - the new ID
// - whether the ID changed
// - whether there was a load error when loading this tre
// - error for other errors (these are errors when saving a tree)
func repairTree(ctx context.Context, opts RepairOptions, repo restic.Repository, path string, treeID *restic.ID, replaces idMap, seen restic.IDSet) (*restic.ID, bool, bool, error) {
	// handle and repair nil trees
	if treeID == nil {
		empty, err := emptyTree(ctx, repo, opts.DryRun)
		Printf("repaired nil tree '%v'\n", path)
		return &empty, true, false, err
	}

	// check if tree was already changed
	newID, ok := replaces[*treeID]
	if ok {
		return &newID, true, false, nil
	}

	// check if tree was seen but not changed
	if seen.Has(*treeID) {
		return treeID, false, false, nil
	}

	tree, err := restic.LoadTree(ctx, repo, *treeID)
	if err != nil {
		// mark as load error
		return &newID, false, true, nil
	}

	var newNodes []*restic.Node
	changed := false
	for _, node := range tree.Nodes {
		switch node.Type {
		case "file":
			ok := true
			var newContent restic.IDs
			var newSize uint64
			// check all contents and remove if not available
			for _, id := range node.Content {
				if size, found := repo.LookupBlobSize(id, restic.DataBlob); !found {
					ok = false
				} else {
					newContent = append(newContent, id)
					newSize += uint64(size)
				}
			}
			if !ok {
				changed = true
				if opts.Append == "" || newSize == 0 {
					Printf("removed defective file '%v'\n", path+node.Name)
					continue
				}
				Printf("repaired defective file '%v'", path+node.Name)
				node.Name = node.Name + opts.Append
				Printf(" to '%v'\n", node.Name)
				node.Content = newContent
				node.Size = newSize
			}
		case "dir":
			// rewrite if necessary
			newID, c, lErr, err := repairTree(ctx, opts, repo, path+node.Name+"/", node.Subtree, replaces, seen)
			switch {
			case err != nil:
				return newID, true, false, err
			case lErr:
				// If we get an error, we remove this subtree
				changed = true
				Printf("removed defective dir '%v'", path+node.Name)
				node.Name = node.Name + opts.Append
				Printf("(now empty '%v')\n", node.Name)
				empty, err := emptyTree(ctx, repo, opts.DryRun)
				if err != nil {
					return newID, true, false, err
				}
				node.Subtree = &empty
			case c:
				node.Subtree = newID
				changed = true
			}
		}
		newNodes = append(newNodes, node)
	}

	if !changed {
		seen.Insert(*treeID)
		return treeID, false, false, nil
	}

	tree.Nodes = newNodes

	if !opts.DryRun {
		newID, err = restic.SaveTree(ctx, repo, tree)
		if err != nil {
			return &newID, true, false, err
		}
		Printf("modified tree %v, new id: %v\n", treeID.Str(), newID.Str())
	} else {
		Printf("would have modified tree %v\n", treeID.Str())
	}

	replaces[*treeID] = newID
	return &newID, true, false, nil
}

func emptyTree(ctx context.Context, repo restic.Repository, dryRun bool) (restic.ID, error) {
	if !dryRun {
		return restic.SaveTree(ctx, repo, &restic.Tree{})
	}
	return restic.ID{}, nil
}
