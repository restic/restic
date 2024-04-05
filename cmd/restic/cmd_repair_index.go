package main

import (
	"context"

	"github.com/restic/restic/internal/index"
	"github.com/restic/restic/internal/pack"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/progress"
	"github.com/restic/restic/internal/ui/termstatus"
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
	RunE: func(cmd *cobra.Command, _ []string) error {
		term, cancel := setupTermstatus()
		defer cancel()
		return runRebuildIndex(cmd.Context(), repairIndexOptions, globalOptions, term)
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

func runRebuildIndex(ctx context.Context, opts RepairIndexOptions, gopts GlobalOptions, term *termstatus.Terminal) error {
	ctx, repo, unlock, err := openWithExclusiveLock(ctx, gopts, false)
	if err != nil {
		return err
	}
	defer unlock()

	printer := newTerminalProgressPrinter(gopts.verbosity, term)

	return rebuildIndex(ctx, opts, repo, printer)
}

func rebuildIndex(ctx context.Context, opts RepairIndexOptions, repo *repository.Repository, printer progress.Printer) error {
	var obsoleteIndexes restic.IDs
	packSizeFromList := make(map[restic.ID]int64)
	packSizeFromIndex := make(map[restic.ID]int64)
	removePacks := restic.NewIDSet()

	if opts.ReadAllPacks {
		// get list of old index files but start with empty index
		err := repo.List(ctx, restic.IndexFile, func(id restic.ID, _ int64) error {
			obsoleteIndexes = append(obsoleteIndexes, id)
			return nil
		})
		if err != nil {
			return err
		}
	} else {
		printer.P("loading indexes...\n")
		mi := index.NewMasterIndex()
		err := index.ForAllIndexes(ctx, repo, repo, func(id restic.ID, idx *index.Index, _ bool, err error) error {
			if err != nil {
				printer.E("removing invalid index %v: %v\n", id, err)
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

	printer.P("getting pack files to read...\n")
	err := repo.List(ctx, restic.PackFile, func(id restic.ID, packSize int64) error {
		size, ok := packSizeFromIndex[id]
		if !ok || size != packSize {
			// Pack was not referenced in index or size does not match
			packSizeFromList[id] = packSize
			removePacks.Insert(id)
		}
		if !ok {
			printer.E("adding pack file to index %v\n", id)
		} else if size != packSize {
			printer.E("reindexing pack file %v with unexpected size %v instead of %v\n", id, packSize, size)
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
		printer.E("removing not found pack file %v\n", id)
	}

	if len(packSizeFromList) > 0 {
		printer.P("reading pack files\n")
		bar := printer.NewCounter("packs")
		bar.SetMax(uint64(len(packSizeFromList)))
		invalidFiles, err := repo.CreateIndexFromPacks(ctx, packSizeFromList, bar)
		bar.Done()
		if err != nil {
			return err
		}

		for _, id := range invalidFiles {
			printer.V("skipped incomplete pack file: %v\n", id)
		}
	}

	err = rebuildIndexFiles(ctx, repo, removePacks, obsoleteIndexes, false, printer)
	if err != nil {
		return err
	}
	printer.P("done\n")

	return nil
}
