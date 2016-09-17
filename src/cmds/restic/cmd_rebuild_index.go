package main

import (
	"restic/repository"

	"github.com/spf13/cobra"
)

var cmdRebuildIndex = &cobra.Command{
	Use:   "rebuild-index [flags]",
	Short: "build a new index file",
	Long: `
The "rebuild-index" command creates a new index by combining the index files
into a new one.
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

	return repository.RebuildIndex(repo)
}
