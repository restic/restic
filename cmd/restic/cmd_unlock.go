package main

import (
	"context"

	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func newUnlockCommand(globalOptions *GlobalOptions) *cobra.Command {
	var opts UnlockOptions

	cmd := &cobra.Command{
		Use:   "unlock",
		Short: "Remove locks other processes created",
		Long: `
The "unlock" command removes stale locks that have been created by other restic processes.

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
`,
		GroupID:           cmdGroupDefault,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runUnlock(cmd.Context(), opts, *globalOptions, globalOptions.Term)
		},
	}
	opts.AddFlags(cmd.Flags())
	return cmd
}

// UnlockOptions collects all options for the unlock command.
type UnlockOptions struct {
	RemoveAll bool
}

func (opts *UnlockOptions) AddFlags(f *pflag.FlagSet) {
	f.BoolVar(&opts.RemoveAll, "remove-all", false, "remove all locks, even non-stale ones")
}

func runUnlock(ctx context.Context, opts UnlockOptions, gopts GlobalOptions, term ui.Terminal) error {
	printer := ui.NewProgressPrinter(gopts.JSON, gopts.Verbosity, term)
	repo, err := OpenRepository(ctx, gopts, printer)
	if err != nil {
		return err
	}

	fn := repository.RemoveStaleLocks
	if opts.RemoveAll {
		fn = repository.RemoveAllLocks
	}

	processed, err := fn(ctx, repo)
	if err != nil {
		return err
	}

	if processed > 0 {
		printer.P("successfully removed %d locks", processed)
	}
	return nil
}
