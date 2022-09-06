package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/walker"
)

var cmdRewrite = &cobra.Command{
	Use:   "rewrite [flags] [all|snapshotID ...]",
	Short: "Rewrite existing snapshots",
	Long: `
The "rewrite" command excludes files from existing snapshots.

By default 'rewrite' will create new snapshots that will contains same data as
the source snapshots but without excluded files. All metadata (time, host, tags)
will be preserved. The special tag 'rewrite' will be added to new snapshots to
distinguish it from the source (unless --inplace is used).

If --inplace option is used, old snapshot will be removed from repository.

Snapshots to rewrite are specified using --host, --tag, --path or by providing
a list of snapshot ids. Alternatively it's possible to use special snapshot id 'all'
that will match all snapshots.

Please note, that this command only creates new snapshots. In order to delete
data from the repository use 'prune' command.

EXIT STATUS
===========

Exit status is 0 if the command was successful, and non-zero if there was any error.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRewrite(cmd.Context(), rewriteOptions, globalOptions, args)
	},
}

// RewriteOptions collects all options for the rewrite command.
type RewriteOptions struct {
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

	filteredTree, err := walker.FilterTree(ctx, repo, "/", *sn.Tree, &walker.TreeFilterVisitor{
		CheckExclude: checkExclude,
		PrintExclude: func(path string) { Verbosef(fmt.Sprintf("excluding %s\n", path)) },
	})

	if err != nil {
		return false, err
	}

	if filteredTree == *sn.Tree {
		debug.Log("Snapshot %v not modified", sn)
		return false, nil
	}

	debug.Log("Snapshot %v modified", sn)
	if opts.DryRun {
		Printf("Would modify snapshot: %s\n", sn.String())
		return true, nil
	}

	err = repo.Flush(ctx)
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
	id, err := restic.SaveSnapshot(ctx, repo, sn)
	if err != nil {
		return false, err
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

func runRewrite(ctx context.Context, opts RewriteOptions, gopts GlobalOptions, args []string) error {

	if len(opts.Hosts) == 0 && len(opts.Tags) == 0 && len(opts.Paths) == 0 && len(args) == 0 {
		return errors.Fatal("no snapshots provided")
	}

	if len(opts.ExcludeFiles) == 0 && len(opts.Excludes) == 0 && len(opts.InsensitiveExcludes) == 0 {
		return errors.Fatal("Nothing to do: no excludes provided")
	}

	if len(args) == 1 && args[0] == "all" {
		args = []string{}
	}

	repo, err := OpenRepository(ctx, gopts)
	if err != nil {
		return err
	}

	if !opts.DryRun {
		Verbosef("create exclusive lock for repository\n")
		var lock *restic.Lock
		lock, ctx, err = lockRepoExclusive(ctx, repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	} else {
		repo.SetDryRun()
	}

	snapshotLister, err := backend.MemorizeList(ctx, repo.Backend(), restic.SnapshotFile)
	if err != nil {
		return err
	}

	if err = repo.LoadIndex(ctx); err != nil {
		return err
	}

	changedCount := 0
	for sn := range FindFilteredSnapshots(ctx, snapshotLister, repo, opts.Hosts, opts.Tags, opts.Paths, args) {
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
			Verbosef("dry run. would modify %v snapshots\n", changedCount)
		}
	}

	return nil
}
