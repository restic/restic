package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/walker"
)

var cmdRewrite = &cobra.Command{
	Use:   "rewrite [flags] [snapshotID ...]",
	Short: "Rewrite snapshots to exclude unwanted files",
	Long: `
The "rewrite" command excludes files from existing snapshots. It creates new
snapshots containing the same data as the original ones, but without the files
you specify to exclude. All metadata (time, host, tags) will be preserved.

The snapshots to rewrite are specified using the --host, --tag and --path options,
or by providing a list of snapshot IDs. Please note that specifying neither any of
these options nor a snapshot ID will cause the command to rewrite all snapshots.

The special tag 'rewrite' will be added to the new snapshots to distinguish
them from the original ones, unless --forget is used. If the --forget option is
used, the original snapshots will instead be directly removed from the repository.

Please note that the --forget option only removes the snapshots and not the actual
data stored in the repository. In order to delete the no longer referenced data,
use the "prune" command.

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
	Forget bool
	DryRun bool

	restic.SnapshotFilter
	excludePatternOptions
}

var rewriteOptions RewriteOptions

func init() {
	cmdRoot.AddCommand(cmdRewrite)

	f := cmdRewrite.Flags()
	f.BoolVarP(&rewriteOptions.Forget, "forget", "", false, "remove original snapshots after creating new ones")
	f.BoolVarP(&rewriteOptions.DryRun, "dry-run", "n", false, "do not do anything, just print what would be done")

	initMultiSnapshotFilter(f, &rewriteOptions.SnapshotFilter, true)
	initExcludePatternOptions(f, &rewriteOptions.excludePatternOptions)
}

func rewriteSnapshot(ctx context.Context, repo *repository.Repository, sn *restic.Snapshot, opts RewriteOptions) (bool, error) {
	if sn.Tree == nil {
		return false, errors.Errorf("snapshot %v has nil tree", sn.ID().Str())
	}

	rejectByNameFuncs, err := opts.excludePatternOptions.CollectPatterns()
	if err != nil {
		return false, err
	}

	selectByName := func(nodepath string) bool {
		for _, reject := range rejectByNameFuncs {
			if reject(nodepath) {
				return false
			}
		}
		return true
	}

	wg, wgCtx := errgroup.WithContext(ctx)
	repo.StartPackUploader(wgCtx, wg)

	var filteredTree restic.ID
	wg.Go(func() error {
		filteredTree, err = walker.FilterTree(wgCtx, repo, "/", *sn.Tree, &walker.TreeFilterVisitor{
			SelectByName: selectByName,
			PrintExclude: func(path string) { Verbosef(fmt.Sprintf("excluding %s\n", path)) },
		})
		if err != nil {
			return err
		}

		return repo.Flush(wgCtx)
	})
	err = wg.Wait()
	if err != nil {
		return false, err
	}

	if filteredTree == *sn.Tree {
		debug.Log("Snapshot %v not modified", sn)
		return false, nil
	}

	debug.Log("Snapshot %v modified", sn)
	if opts.DryRun {
		Verbosef("would save new snapshot\n")

		if opts.Forget {
			Verbosef("would remove old snapshot\n")
		}

		return true, nil
	}

	// Always set the original snapshot id as this essentially a new snapshot.
	sn.Original = sn.ID()
	*sn.Tree = filteredTree

	if !opts.Forget {
		sn.AddTags([]string{"rewrite"})
	}

	// Save the new snapshot.
	id, err := restic.SaveSnapshot(ctx, repo, sn)
	if err != nil {
		return false, err
	}

	if opts.Forget {
		h := restic.Handle{Type: restic.SnapshotFile, Name: sn.ID().String()}
		if err = repo.Backend().Remove(ctx, h); err != nil {
			return false, err
		}
		debug.Log("removed old snapshot %v", sn.ID())
		Verbosef("removed old snapshot %v\n", sn.ID().Str())
	}
	Verbosef("saved new snapshot %v\n", id.Str())
	return true, nil
}

func runRewrite(ctx context.Context, opts RewriteOptions, gopts GlobalOptions, args []string) error {
	if opts.excludePatternOptions.Empty() {
		return errors.Fatal("Nothing to do: no excludes provided")
	}

	repo, err := OpenRepository(ctx, gopts)
	if err != nil {
		return err
	}

	if !opts.DryRun {
		var lock *restic.Lock
		var err error
		if opts.Forget {
			Verbosef("create exclusive lock for repository\n")
			lock, ctx, err = lockRepoExclusive(ctx, repo)
		} else {
			lock, ctx, err = lockRepo(ctx, repo)
		}
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
	for sn := range FindFilteredSnapshots(ctx, snapshotLister, repo, &opts.SnapshotFilter, args) {
		Verbosef("\nsnapshot %s of %v at %s)\n", sn.ID().Str(), sn.Paths, sn.Time)
		changed, err := rewriteSnapshot(ctx, repo, sn, opts)
		if err != nil {
			return errors.Fatalf("unable to rewrite snapshot ID %q: %v", sn.ID().Str(), err)
		}
		if changed {
			changedCount++
		}
	}

	Verbosef("\n")
	if changedCount == 0 {
		if !opts.DryRun {
			Verbosef("no snapshots were modified\n")
		} else {
			Verbosef("no snapshots would be modified\n")
		}
	} else {
		if !opts.DryRun {
			Verbosef("modified %v snapshots\n", changedCount)
		} else {
			Verbosef("would modify %v snapshots\n", changedCount)
		}
	}

	return nil
}
