package main

import (
	"context"
	"path"

	"github.com/spf13/cobra"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
)

var cmdRewrite = &cobra.Command{
	Use:   "rewrite [f] [all|snapshotID ...]",
	Short: "Modify existing snapshots by deleting files",
	Long: `
The "rewrite" command excludes files from existing snapshots.

By default 'rewrite' will create new snapshot that will contains same data as
source snapshot except excluded data. All metadata (time, host, tags) will be preserved.
Special tag 'rewrite' will be added to new snapshot to distinguish it from source
(unless --inplace is used)

If --inplace option is used, old snapshot will be removed from repository.

Snapshots to rewrite are specified using --host, --tag, --path or by providing list of snapshotID.
Alternatively it's possible to use special 'all' snapshot that will match all snapshots

Please note, that this command only modifies snapshot objects. In order to delete
data from repository use 'prune' command

EXIT STATUS
===========

Exit status is 0 if the command was successful, and non-zero if there was any error.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRewrite(rewriteOptions, globalOptions, args)
	},
}

// RewriteOptions collects all options for the ls command.
type RewriteOptions struct {
	// TagOptions bundles all options for the 'tag' command.
	Hosts   []string
	Paths   []string
	Tags    restic.TagLists
	Inplace bool
	DryRun  bool

	// Exclude options
	Excludes            []string
	InsensitiveExcludes []string
	ExcludeFiles        []string
}

var rewriteOptions RewriteOptions

func init() {
	cmdRoot.AddCommand(cmdRewrite)

	f := cmdRewrite.Flags()
	f.StringArrayVarP(&rewriteOptions.Hosts, "host", "H", nil, "only consider snapshots for this `host`, when no snapshot ID is given (can be specified multiple times)")
	f.Var(&rewriteOptions.Tags, "tag", "only consider snapshots which include this `taglist`, when no snapshot-ID is given")
	f.StringArrayVar(&rewriteOptions.Paths, "path", nil, "only consider snapshots which include this (absolute) `path`, when no snapshot-ID is given")
	f.BoolVarP(&rewriteOptions.Inplace, "inplace", "", false, "replace existing snapshots")
	f.BoolVarP(&rewriteOptions.DryRun, "dry-run", "n", false, "do not do anything, just print what would be done")

	// Excludes
	f.StringArrayVarP(&rewriteOptions.Excludes, "exclude", "e", nil, "exclude a `pattern` (can be specified multiple times)")
	f.StringArrayVar(&rewriteOptions.InsensitiveExcludes, "iexclude", nil, "same as --exclude `pattern` but ignores the casing of filenames")
	f.StringArrayVar(&rewriteOptions.ExcludeFiles, "exclude-file", nil, "read exclude patterns from a `file` (can be specified multiple times)")
}

type saveTreeFunction = func(*restic.Tree) (restic.ID, error)

func filterNode(ctx context.Context, repo restic.Repository, nodepath string, nodeID restic.ID, checkExclude RejectByNameFunc, saveTreeFunc saveTreeFunction) (newNodeID restic.ID, err error) {
	curTree, err := repo.LoadTree(ctx, nodeID)
	if err != nil {
		return nodeID, err
	}

	debug.Log("filterNode: %s, nodeId: %s\n", nodepath, nodeID.Str())

	changed := false
	newTree := restic.NewTree()
	for _, node := range curTree.Nodes {
		path := path.Join(nodepath, node.Name)
		if !checkExclude(path) {
			if node.Subtree == nil {
				newTree.Insert(node)
				continue
			}
			newNode := node
			newID, err := filterNode(ctx, repo, path, *node.Subtree, checkExclude, saveTreeFunc)
			if err != nil {
				return nodeID, err
			}
			if newID == *node.Subtree {
				newTree.Insert(node)
			} else {
				changed = true
				newNode.Subtree = new(restic.ID)
				*newNode.Subtree = newID
				newTree.Insert(newNode)
			}
		} else {
			Verbosef("excluding %s\n", path)
			changed = true
		}
	}

	if changed {
		// Save new tree
		newTreeID, err := saveTreeFunc(newTree)
		debug.Log("filterNode: save new tree for %s as %v\n", nodepath, newTreeID)
		return newTreeID, err
	}

	return nodeID, nil
}

func collectRejectFuncsForRewrite(opts RewriteOptions) (fs []RejectByNameFunc, err error) {
	//TODO: merge with cmd_backup

	// add patterns from file
	if len(opts.ExcludeFiles) > 0 {
		excludes, err := readExcludePatternsFromFiles(opts.ExcludeFiles)
		if err != nil {
			return nil, err
		}
		opts.Excludes = append(opts.Excludes, excludes...)
	}

	if len(opts.InsensitiveExcludes) > 0 {
		fs = append(fs, rejectByInsensitivePattern(opts.InsensitiveExcludes))
	}

	if len(opts.Excludes) > 0 {
		fs = append(fs, rejectByPattern(opts.Excludes))
	}

	return fs, nil
}

func rewriteSnapshot(ctx context.Context, repo *repository.Repository, sn *restic.Snapshot, opts RewriteOptions, gopts GlobalOptions) (bool, error) {
	if sn.Tree == nil {
		return false, errors.Errorf("snapshot %v has nil tree", sn.ID().Str())
	}

	var saveTreeFunc saveTreeFunction
	if !opts.DryRun {
		saveTreeFunc = func(tree *restic.Tree) (restic.ID, error) {
			return repo.SaveTree(ctx, tree)
		}
	} else {
		saveTreeFunc = func(tree *restic.Tree) (restic.ID, error) {
			return restic.ID{0}, nil
		}
	}

	rejectByNameFuncs, err := collectRejectFuncsForRewrite(opts)
	if err != nil {
		return false, err
	}

	checkExclude := func(nodepath string) bool {
		for _, reject := range rejectByNameFuncs {
			if reject(nodepath) {
				return true
			}
		}
		return false
	}

	filteredTree, err := filterNode(ctx, repo, "/", *sn.Tree, checkExclude, saveTreeFunc)

	if err != nil {
		return false, err
	}

	if filteredTree == *sn.Tree {
		debug.Log("Snapshot not touched\n")
		return false, nil
	}

	debug.Log("Snapshot modified\n")
	if opts.DryRun {
		Printf("Will modify snapshot: %s\n", sn.String())
		return true, nil
	}

	err = repo.Flush(ctx)
	if err != nil {
		return false, err
	}
	err = repo.SaveIndex(ctx)
	if err != nil {
		return false, err
	}

	// Retain the original snapshot id over all tag changes.
	if sn.Original == nil {
		sn.Original = sn.ID()
	}
	*sn.Tree = filteredTree

	if !opts.Inplace {
		sn.AddTags([]string{"rewrite"})
	}

	// Save the new snapshot.
	id, err := repo.SaveJSONUnpacked(ctx, restic.SnapshotFile, sn)
	if err != nil {
		return false, err
	}
	err = repo.Flush(ctx)
	if err != nil {
		return true, err
	}

	if opts.Inplace {
		h := restic.Handle{Type: restic.SnapshotFile, Name: sn.ID().String()}
		if err = repo.Backend().Remove(ctx, h); err != nil {
			return false, err
		}

		debug.Log("old snapshot %v removed", sn.ID())
	}
	Printf("new snapshot saved as %v\n", id)
	return true, nil
}

func runRewrite(opts RewriteOptions, gopts GlobalOptions, args []string) error {

	if len(opts.Hosts) == 0 && len(opts.Tags) == 0 && len(opts.Paths) == 0 && len(args) == 0 {
		return errors.Fatal("no snapshots provided")
	}

	if len(opts.ExcludeFiles) == 0 && len(opts.Excludes) == 0 && len(opts.InsensitiveExcludes) == 0 {
		return errors.Fatal("Nothing to do: no excludes provided")
	}

	if len(args) == 1 && args[0] == "all" {
		args = []string{}
	}

	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	if !gopts.NoLock && !opts.DryRun {
		Verbosef("create exclusive lock for repository\n")
		lock, err := lockRepoExclusive(repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	if err = repo.LoadIndex(gopts.ctx); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(gopts.ctx)
	defer cancel()

	changedCount := 0
	for sn := range FindFilteredSnapshots(ctx, repo, opts.Hosts, opts.Tags, opts.Paths, args) {
		Verbosef("Checking snapshot %s\n", sn.String())
		changed, err := rewriteSnapshot(ctx, repo, sn, opts, gopts)
		if err != nil {
			Warnf("unable to rewrite snapshot ID %q, ignoring: %v\n", sn.ID(), err)
			continue
		}
		if changed {
			changedCount++
		}
	}

	if changedCount == 0 {
		Verbosef("no snapshots modified\n")
	} else {
		if !opts.DryRun {
			Verbosef("modified %v snapshots\n", changedCount)
		} else {
			Verbosef("dry run. %v snapshots affected\n", changedCount)
		}
	}

	return nil
}
