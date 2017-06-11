package main

import (
	"context"
	"restic"
	"restic/cache"
	"restic/walk"

	"github.com/spf13/cobra"
)

var cmdCache = &cobra.Command{
	Use:   "cache [name]",
	Short: "update the cache migration",
	Long: `
The "cache" command updates the cache.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCache(cacheOptions, globalOptions, args)
	},
}

// CacheOptions bundles all options for the 'check' command.
type CacheOptions struct {
}

var cacheOptions CacheOptions

func init() {
	cmdRoot.AddCommand(cmdCache)
}

func writeSnapshotCache(ctx context.Context, repo restic.Repository, cache restic.Cache, id restic.ID) error {
	sn, err := restic.LoadSnapshot(ctx, repo, id)
	if err != nil {
		return err
	}

	wr, err := cache.NewSnapshotWriter(id, sn)
	if err != nil {
		return err
	}

	err = walk.Walk(ctx, repo, *sn.Tree, func(dir string, id restic.ID, tree *restic.Tree, err error) error {
		if err != nil {
			return err
		}
		Printf("   %v\n", dir)
		return wr.Add(dir, id, tree)
	})

	e := wr.Close()
	if err == nil {
		err = e
	}

	return err
}

func runCache(opts CacheOptions, gopts GlobalOptions, args []string) error {
	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	if err := repo.LoadIndex(gopts.ctx); err != nil {
		return err
	}

	lock, err := lockRepo(repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	cache, err := cache.New(repo.Config().ID, "", repo, repo.Key())
	if err != nil {
		return err
	}

	Printf("updating cache for snapshots:\n")
	for id := range repo.List(gopts.ctx, restic.SnapshotFile) {
		Printf("  snapshot %v\n", id.Str())
		if err := writeSnapshotCache(gopts.ctx, repo, cache, id); err != nil {
			return err
		}
	}

	return nil
}
