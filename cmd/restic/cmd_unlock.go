package main

import (
	"context"

	"github.com/restic/restic/internal/restic"
	"github.com/spf13/cobra"
)

var unlockCmd = &cobra.Command{
	Use:   "unlock",
	Short: "Remove locks other processes created",
	Long: `
The "unlock" command removes stale locks that have been created by other restic processes.

EXIT STATUS
===========

Exit status is 0 if the command was successful, and non-zero if there was any error.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runUnlock(cmd.Context(), unlockOptions, globalOptions)
	},
}

// UnlockOptions collects all options for the unlock command.
type UnlockOptions struct {
	RemoveAll bool
}

var unlockOptions UnlockOptions

func init() {
	cmdRoot.AddCommand(unlockCmd)

	unlockCmd.Flags().BoolVar(&unlockOptions.RemoveAll, "remove-all", false, "remove all locks, even non-stale ones")
}

func runUnlock(ctx context.Context, opts UnlockOptions, gopts GlobalOptions) error {
	repo, err := OpenRepository(ctx, gopts)
	if err != nil {
		return err
	}

	fn := restic.RemoveStaleLocks
	if opts.RemoveAll {
		fn = restic.RemoveAllLocks
	}

	processed, err := fn(ctx, repo)
	if err != nil {
		return err
	}

	if processed > 0 {
		Verbosef("successfully removed %d locks\n", processed)
	}
	return nil
}
