package main

import (
	"restic"
	"restic/index"

	"github.com/spf13/cobra"
)

var cmdRebuildIndex = &cobra.Command{
	Use:   "rebuild-index [flags]",
	Short: "build a new index file",
	Long: `
The "rebuild-index" command creates a new index based on the pack files in the
repository.
`,
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

	done := make(chan struct{})
	defer close(done)

	Verbosef("counting files in repo\n")

	var packs uint64
	for _ = range repo.List(restic.DataFile, done) {
		packs++
	}

	bar := newProgressMax(!gopts.Quiet, packs, "packs")
	idx, err := index.New(repo, bar)
	if err != nil {
		return err
	}

	Verbosef("listing old index files\n")
	var supersedes restic.IDs
	for id := range repo.List(restic.IndexFile, done) {
		supersedes = append(supersedes, id)
	}

	id, err := idx.Save(repo, supersedes)
	if err != nil {
		return err
	}

	Verbosef("saved new index as %v\n", id.Str())

	Verbosef("remove %d old index files\n", len(supersedes))

	for _, id := range supersedes {
		err := repo.Backend().Remove(restic.Handle{
			Type: restic.IndexFile,
			Name: id.String(),
		})

		if err != nil {
			Warnf("error deleting old index %v: %v\n", id.Str(), err)
		}
	}

	return nil
}
