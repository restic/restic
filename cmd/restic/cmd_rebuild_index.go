package main

import (
	"context"

	"github.com/restic/restic/internal/index"
	"github.com/restic/restic/internal/pack"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"

	"github.com/spf13/cobra"
)

var cmdRebuildIndex = &cobra.Command{
	Use:   "rebuild-index [flags]",
	Short: "Build a new index",
	Long: `
The "rebuild-index" command creates a new index based on the pack files in the
repository.

EXIT STATUS
===========

Exit status is 0 if the command was successful, and non-zero if there was any error.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRebuildIndex(cmd.Context(), rebuildIndexOptions, globalOptions)
	},
}

// RebuildIndexOptions collects all options for the rebuild-index command.
type RebuildIndexOptions struct {
	ReadAllPacks bool
}

var rebuildIndexOptions RebuildIndexOptions

func init() {
	cmdRoot.AddCommand(cmdRebuildIndex)
	f := cmdRebuildIndex.Flags()
	f.BoolVar(&rebuildIndexOptions.ReadAllPacks, "read-all-packs", false, "read all pack files to generate new index from scratch")

}

func runRebuildIndex(ctx context.Context, opts RebuildIndexOptions, gopts GlobalOptions) error {
	repo, err := OpenRepository(ctx, gopts)
	if err != nil {
		return err
	}

	lock, ctx, err := lockRepoExclusive(ctx, repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	return rebuildIndex(ctx, opts, gopts, repo, restic.NewIDSet())
}

func rebuildIndex(ctx context.Context, opts RebuildIndexOptions, gopts GlobalOptions, repo *repository.Repository, ignorePacks restic.IDSet) error {
	var obsoleteIndexes restic.IDs
	packSizeFromList := make(map[restic.ID]int64)
	packSizeFromIndex := make(map[restic.ID]int64)
	removePacks := restic.NewIDSet()

	if opts.ReadAllPacks {
		// get list of old index files but start with empty index
		err := repo.List(ctx, restic.IndexFile, func(id restic.ID, size int64) error {
			obsoleteIndexes = append(obsoleteIndexes, id)
			return nil
		})
		if err != nil {
			return err
		}
	} else {
		Verbosef("loading indexes...\n")
		mi := index.NewMasterIndex()
		err := index.ForAllIndexes(ctx, repo, func(id restic.ID, idx *index.Index, oldFormat bool, err error) error {
			if err != nil {
				Warnf("removing invalid index %v: %v\n", id, err)
				obsoleteIndexes = append(obsoleteIndexes, id)
				return nil
			}

			mi.Insert(idx)
			return nil
		})
		if err != nil {
			return err
		}

		err = mi.MergeFinalIndexes()
		if err != nil {
			return err
		}

		err = repo.SetIndex(mi)
		if err != nil {
			return err
		}
		packSizeFromIndex = pack.Size(ctx, repo.Index(), false)
	}

	Verbosef("getting pack files to read...\n")
	err := repo.List(ctx, restic.PackFile, func(id restic.ID, packSize int64) error {
		size, ok := packSizeFromIndex[id]
		if !ok || size != packSize {
			// Pack was not referenced in index or size does not match
			packSizeFromList[id] = packSize
			removePacks.Insert(id)
		}
		if !ok {
			Warnf("adding pack file to index %v\n", id)
		} else if size != packSize {
			Warnf("reindexing pack file %v with unexpected size %v instead of %v\n", id, packSize, size)
		}
		delete(packSizeFromIndex, id)
		return nil
	})
	if err != nil {
		return err
	}
	for id := range packSizeFromIndex {
		// forget pack files that are referenced in the index but do not exist
		// when rebuilding the index
		removePacks.Insert(id)
		Warnf("removing not found pack file %v\n", id)
	}

	if len(packSizeFromList) > 0 {
		Verbosef("reading pack files\n")
		bar := newProgressMax(!globalOptions.Quiet, uint64(len(packSizeFromList)), "packs")
		invalidFiles, err := repo.CreateIndexFromPacks(ctx, packSizeFromList, bar)
		bar.Done()
		if err != nil {
			return err
		}

		for _, id := range invalidFiles {
			Verboseff("skipped incomplete pack file: %v\n", id)
		}
	}

	err = rebuildIndexFiles(ctx, gopts, repo, removePacks, obsoleteIndexes)
	if err != nil {
		return err
	}
	Verbosef("done\n")

	return nil
}
