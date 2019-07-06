package main

import (
	"context"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/index"
	"github.com/restic/restic/internal/restic"

	"github.com/spf13/cobra"
)

var cmdRebuildIndex = &cobra.Command{
	Use:   "rebuild-index [flags]",
	Short: "Build a new index file",
	Long: `
The "rebuild-index" command creates a new index based on the pack files in the
repository.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRebuildIndex(globalOptions)
	},
}

func init() {
	cmdRoot.AddCommand(cmdRebuildIndex)
}

func runRebuildIndex(gopts GlobalOptions) error {
	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	lock, err := lockRepoExclusive(repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(gopts.ctx)
	defer cancel()
	return rebuildIndex(ctx, repo, restic.NewIDSet())
}

func rebuildIndex(ctx context.Context, repo restic.Repository, ignorePacks restic.IDSet) error {
	Verbosef("counting files in repo\n")

	var packs uint64
	err := repo.List(ctx, restic.DataFile, func(restic.ID, int64) error {
		packs++
		return nil
	})
	if err != nil {
		return err
	}

	bar := newProgressMax(!globalOptions.Quiet, packs-uint64(len(ignorePacks)), "packs")
	idx, _, err := index.New(ctx, repo, ignorePacks, bar)
	if err != nil {
		return err
	}

	Verbosef("finding old index files\n")

	var supersedes restic.IDs
	err = repo.List(ctx, restic.IndexFile, func(id restic.ID, size int64) error {
		supersedes = append(supersedes, id)
		return nil
	})
	if err != nil {
		return err
	}

	ids, err := idx.Save(ctx, repo, supersedes, nil)
	if err != nil {
		return errors.Fatalf("unable to save index, last error was: %v", err)
	}

	Verbosef("saved new indexes as %v\n", ids)

	Verbosef("remove %d old index files\n", len(supersedes))

	for _, id := range supersedes {
		if err := repo.Backend().Remove(ctx, restic.Handle{
			Type: restic.IndexFile,
			Name: id.String(),
		}); err != nil {
			Warnf("error removing old index %v: %v\n", id.Str(), err)
		}
	}

	return nil
}
