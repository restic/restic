package main

import (
	"github.com/restic/restic/internal/index"
	"github.com/restic/restic/internal/restic"

	"github.com/spf13/cobra"
)

var cmdRepackIndex = &cobra.Command{
	Use:   "repack-index [flags]",
	Short: "repack index files",
	Long: `
The "repack-index" command repacks index files, that is putting
index entries of small files together in larger index files.`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRepackIndex(repackIndexOptions, globalOptions)
	},
}

// RepackIndexOptions collects all options for the repack-index command.
type RepackIndexOptions struct {
	DryRun bool
}

var repackIndexOptions RepackIndexOptions

func init() {
	cmdRoot.AddCommand(cmdRepackIndex)

	f := cmdRepackIndex.Flags()
	f.BoolVarP(&repackIndexOptions.DryRun, "dry-run", "n", false, "do not delete anything, just print what would be done")
}

func runRepackIndex(opts RepackIndexOptions, gopts GlobalOptions) error {
	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	lock, err := lockRepoExclusive(repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	err = RepackIndex(opts, gopts, repo)
	if err != nil {
		return err
	}
	return nil
}

func RepackIndex(opts RepackIndexOptions, gopts GlobalOptions, repo restic.Repository) error {
	Verbosef("load all index files\n")
	var supersedes restic.IDs
	err := repo.List(gopts.ctx, restic.IndexFile, func(id restic.ID, size int64) error {
		supersedes = append(supersedes, id)
		return nil
	})
	if err != nil {
		return err
	}

	bar := newProgressMax(!gopts.Quiet, uint64(len(supersedes)), "index files loaded")
	bar.Start()
	idx, err := index.Load(gopts.ctx, repo, bar)
	if err != nil {
		return err
	}

	if !opts.DryRun {
		Verbosef("saving index\n")
		_, err = idx.Save(gopts.ctx, repo, nil)
		if err != nil {
			return err
		}

		Verbosef("remove %d old index files\n", len(supersedes))
		for _, id := range supersedes {
			if err := repo.Backend().Remove(gopts.ctx, restic.Handle{
				Type: restic.IndexFile,
				Name: id.String(),
			}); err != nil {
				Warnf("error removing old index %v: %v\n", id.Str(), err)
			}
		}
	} else {
		Verbosef("would have replaced %d index files\n", len(supersedes))
	}

	return nil

}
