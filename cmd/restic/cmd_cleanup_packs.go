package main

import (
	"github.com/restic/restic/internal/restic"

	"github.com/spf13/cobra"
)

var cmdCleanupPacks = &cobra.Command{
	Use:   "cleanup-packs [flags]",
	Short: "Remove packs not in index",
	Long: `
The "cleanup-packs" command removes packs
that are not contained in any index files.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCleanupPacks(cleanupPacksOptions, globalOptions)
	},
}

// CleanupIndexOptions collects all options for the cleanup-index command.
type CleanupPacksOptions struct {
	DryRun bool
}

var cleanupPacksOptions CleanupPacksOptions

func init() {
	cmdRoot.AddCommand(cmdCleanupPacks)

	f := cmdCleanupPacks.Flags()
	f.BoolVarP(&cleanupPacksOptions.DryRun, "dry-run", "n", false, "do not delete anything, just print what would be done")
}

func runCleanupPacks(opts CleanupPacksOptions, gopts GlobalOptions) error {
	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	lock, err := lockRepoExclusive(repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	Verbosef("load indexes\n")
	err = repo.LoadIndex(gopts.ctx)
	if err != nil {
		return err
	}

	return CleanupPacks(opts, gopts, repo)
}

func CleanupPacks(opts CleanupPacksOptions, gopts GlobalOptions, repo restic.Repository) error {

	ctx := gopts.ctx

	Verbosef("find blobs in index\n")
	packs := restic.NewIDSet()
	for blob := range repo.Index().Each(ctx) {
		packs.Insert(blob.PackID)
	}

	Verbosef("collect packs for deletion\n")
	removePacks := restic.NewIDSet()
	removeBytes := uint64(0)
	// TODO: Add parallel processing
	err := repo.List(ctx, restic.DataFile, func(id restic.ID, size int64) error {
		if !packs.Has(id) {
			removePacks.Insert(id)
			removeBytes += uint64(size)
		}
		return nil
	})
	if err != nil {
		return err
	}

	Verbosef("will delete %d packs\n", len(removePacks))
	Verbosef("frees %s\n", formatBytes(removeBytes))

	// TODO: Add parallel processing
	if len(removePacks) != 0 {
		bar := newProgressMax(!gopts.Quiet, uint64(len(removePacks)), "packs deleted")
		bar.Start()
		for packID := range removePacks {
			if !opts.DryRun {
				h := restic.Handle{Type: restic.DataFile, Name: packID.String()}
				err = repo.Backend().Remove(ctx, h)
				if err != nil {
					Warnf("unable to remove file %v from the repository\n", packID.Str())
				}
				if !gopts.JSON {
					Verbosef("pack %v was removed.\n", packID.Str())
				}
			} else {
				if !gopts.JSON {
					Verbosef("would have removed pack %v.\n", packID.Str())
				}
			}

			bar.Report(restic.Stat{Blobs: 1})
		}
		bar.Done()
	}

	Verbosef("done\n")
	return nil
}
