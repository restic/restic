package main

import (
	"context"

	"github.com/restic/restic/internal/index"
	"github.com/restic/restic/internal/pack"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var cmdRepairIndex = &cobra.Command{
	Use:   "index [flags]",
	Short: "Build a new index",
	Long: `
The "repair index" command creates a new index based on the pack files in the
repository.

EXIT STATUS
===========

Exit status is 0 if the command was successful, and non-zero if there was any error.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRebuildIndex(cmd.Context(), repairIndexOptions, globalOptions)
	},
}

var cmdRebuildIndex = &cobra.Command{
	Use:               "rebuild-index [flags]",
	Short:             cmdRepairIndex.Short,
	Long:              cmdRepairIndex.Long,
	Deprecated:        `Use "repair index" instead`,
	DisableAutoGenTag: true,
	RunE:              cmdRepairIndex.RunE,
}

// RepairIndexOptions collects all options for the repair index command.
type RepairIndexOptions struct {
	ReadAllPacks bool
}

var repairIndexOptions RepairIndexOptions

func init() {
	cmdRepair.AddCommand(cmdRepairIndex)
	// add alias for old name
	cmdRoot.AddCommand(cmdRebuildIndex)

	for _, f := range []*pflag.FlagSet{cmdRepairIndex.Flags(), cmdRebuildIndex.Flags()} {
		f.BoolVar(&repairIndexOptions.ReadAllPacks, "read-all-packs", false, "read all pack files to generate new index from scratch")
	}
}

func runRebuildIndex(ctx context.Context, opts RepairIndexOptions, gopts GlobalOptions) error {
	repo, err := OpenRepository(ctx, gopts)
	if err != nil {
		return err
	}

	lock, ctx, err := lockRepoExclusive(ctx, repo, gopts.RetryLock, gopts.JSON)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	return rebuildIndex(ctx, opts, gopts, repo)
}

func rebuildIndex(ctx context.Context, opts RepairIndexOptions, gopts GlobalOptions, repo *repository.Repository) error {
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
		bar := newProgressMax(!gopts.Quiet, uint64(len(packSizeFromList)), "packs")
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
