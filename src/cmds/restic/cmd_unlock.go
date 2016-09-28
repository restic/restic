package main

import (
	"restic"

	"github.com/spf13/cobra"
)

var unlockCmd = &cobra.Command{
	Use:   "unlock",
	Short: "remove locks other processes created",
	Long: `
The "unlock" command removes stale locks that have been created by other restic processes.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runUnlock(unlockOptions, globalOptions)
	},
}

// UnlockOptions collects all options for the unlock command.
type UnlockOptions struct {
	RemoveAll bool
}

var unlockOptions UnlockOptions

func init() {
	cmdRoot.AddCommand(unlockCmd)

	unlockCmd.Flags().BoolVar(&unlockOptions.RemoveAll, "remove-all", false, "Remove all locks, even non-stale ones")
}

func runUnlock(opts UnlockOptions, gopts GlobalOptions) error {
	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	fn := restic.RemoveStaleLocks
	if opts.RemoveAll {
		fn = restic.RemoveAllLocks
	}

	err = fn(repo)
	if err != nil {
		return err
	}

	Verbosef("successfully removed locks\n")
	return nil
}
