package main

import (
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"

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
		return runRepair(repairOptions, args)
	},
}

// RestoreOptions collects all options for the restore command.
type RepairOptions struct {
	Hosts           []string
	Paths           []string
	Tags            restic.TagLists
	AddTag          string
	Append          string
	DryRun          bool
	DeleteSnapshots bool
}

var repairOptions RepairOptions

func init() {
	cmdRoot.AddCommand(cmdRepair)
	flags := cmdRepair.Flags()
	flags.StringArrayVarP(&repairOptions.Hosts, "host", "H", nil, `only consider snapshots for this host (can be specified multiple times)`)
	flags.Var(&repairOptions.Tags, "tag", "only consider snapshots which include this `taglist`")
	flags.StringArrayVar(&repairOptions.Paths, "path", nil, "only consider snapshots which include this (absolute) `path`")
	flags.StringVar(&repairOptions.AddTag, "add-tag", "repaired", "tag to add to repaired snapshots")
	flags.StringVar(&repairOptions.Append, "append", ".repaired", "string to append to repaired dirs/files; remove files if emtpy or impossible to repair")
	flags.BoolVarP(&repairOptions.DryRun, "dry-run", "n", true, "don't do anything, only show what would be done")
	flags.BoolVar(&repairOptions.DeleteSnapshots, "delete-snapshots", false, "delete original snapshots")
}

func runRepair(opts RepairOptions, args []string) error {
	switch {
	case opts.DryRun:
		Printf("\n note: --dry-run is set\n-> repair will only show what it would do.\n\n")
	case opts.DeleteSnapshots:
		Printf("\n note: --dry-run is not set and --delete is set\n-> this may result in data loss!\n\n")
	}

	repo, err := OpenRepository(globalOptions)
	if err != nil {
		return err
	}

	lock, err := lockRepoExclusive(globalOptions.ctx, repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	if err := repo.LoadIndex(globalOptions.ctx); err != nil {
		return err
	}

	// get snapshots to check & repair
	var snapshots []*restic.Snapshot
	for sn := range FindFilteredSnapshots(globalOptions.ctx, repo.Backend(), repo, opts.Hosts, opts.Tags, opts.Paths, args) {
		snapshots = append(snapshots, sn)
	}

	return repairSnapshots(opts, repo, snapshots)
}

func repairSnapshots(opts RepairOptions, repo restic.Repository, snapshots []*restic.Snapshot) error {
	ctx := globalOptions.ctx

	replaces := make(idMap)
	seen := restic.NewIDSet()
	deleteSn := restic.NewIDSet()

	Verbosef("check and repair %d snapshots\n", len(snapshots))
	bar := newProgressMax(!globalOptions.Quiet, uint64(len(snapshots)), "snapshots")
	for _, sn := range snapshots {
		debug.Log("process snapshot %v", sn.ID())
		Printf("%v:\n", sn)
		newID, changed, err := repairTree(opts, repo, "/", *sn.Tree, replaces, seen)
		switch {
		case err != nil:
			Printf("the root tree is damaged -> delete snapshot.\n")
			deleteSn.Insert(*sn.ID())
		case changed:
			err = changeSnapshot(opts, repo, sn, newID)
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

	err := repo.Flush(ctx)
	if err != nil {
		return err
	}

	if len(deleteSn) > 0 && opts.DeleteSnapshots {
		Verbosef("delete %d snapshots...\n", len(deleteSn))
		if !opts.DryRun {
			DeleteFiles(globalOptions, repo, deleteSn, restic.SnapshotFile)
		}
	}
	return nil
}

// changeSnapshot creates a modified snapshot:
// - set the tree to newID
// - add the rag opts.AddTag
// - preserve original ID
// if opts.DryRun is set, it doesn't change anything but only
func changeSnapshot(opts RepairOptions, repo restic.Repository, sn *restic.Snapshot, newID restic.ID) error {
	sn.AddTags([]string{opts.AddTag})
	// Retain the original snapshot id over all tag changes.
	if sn.Original == nil {
		sn.Original = sn.ID()
	}
	sn.Tree = &newID
	if !opts.DryRun {
		newID, err := repo.SaveJSONUnpacked(globalOptions.ctx, restic.SnapshotFile, sn)
		if err != nil {
			return err
		}
		Printf("snapshot repaired -> %v created.\n", newID.Str())
	} else {
		Printf("would have repaired snpshot %v.\n", sn.ID().Str())
	}
	return nil
}

type idMap map[restic.ID]restic.ID

// repairTree checks and repairs a tree and all its subtrees
// Two error cases are checked:
// - trees which cannot be loaded (-> the tree contents will be removed)
// - files whose contents are not fully available  (-> file will be modified)
// In case of an error, the changes made depends on:
// - opts.Append: string to append to "repared" names; if empty files will not repaired but deleted
// - opts.DryRun: if set to true, only print out what to but don't change anything
func repairTree(opts RepairOptions, repo restic.Repository, path string, treeID restic.ID, replaces idMap, seen restic.IDSet) (restic.ID, bool, error) {
	ctx := globalOptions.ctx

	// check if tree was already changed
	newID, ok := replaces[treeID]
	if ok {
		return newID, true, nil
	}

	// check if tree was seen but not changed
	if seen.Has(treeID) {
		return treeID, false, nil
	}

	tree, err := repo.LoadTree(ctx, treeID)
	if err != nil {
		return newID, false, err
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
					Printf("removed defect file '%v'\n", path+node.Name)
					continue
				}
				Printf("repaired defect file '%v'", path+node.Name)
				node.Name = node.Name + opts.Append
				Printf(" to '%v'\n", node.Name)
				node.Content = newContent
				node.Size = newSize
			}
		case "dir":
			// rewrite if necessary
			newID, c, err := repairTree(opts, repo, path+node.Name+"/", *node.Subtree, replaces, seen)
			switch {
			case err != nil:
				// If we get an error, we remove this subtree
				changed = true
				Printf("removed defect dir '%v'", path+node.Name)
				node.Name = node.Name + opts.Append
				Printf("(now emtpy '%v')\n", node.Name)
				node.Subtree = nil
			case c:
				node.Subtree = &newID
				changed = true
			}
		}
		newNodes = append(newNodes, node)
	}

	if !changed {
		seen.Insert(treeID)
		return treeID, false, nil
	}

	tree.Nodes = newNodes

	if !opts.DryRun {
		newID, err = repo.SaveTree(ctx, tree)
		if err != nil {
			return newID, false, err
		}
		Printf("modified tree %v, new id: %v\n", treeID.Str(), newID.Str())
	} else {
		Printf("would have modified tree %v\n", treeID.Str())
	}

	replaces[treeID] = newID
	return newID, true, nil
}
