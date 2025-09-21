package main

import (
	"context"

	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func newRepairIndexCommand(globalOptions *GlobalOptions) *cobra.Command {
	var opts RepairIndexOptions

	cmd := &cobra.Command{
		Use:   "index [flags]",
		Short: "Build a new index",
		Long: `
The "repair index" command creates a new index based on the pack files in the
repository.

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
Exit status is 10 if the repository does not exist.
Exit status is 11 if the repository is already locked.
Exit status is 12 if the password is incorrect.
`,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRebuildIndex(cmd.Context(), opts, *globalOptions, globalOptions.term)
		},
	}

	opts.AddFlags(cmd.Flags())
	return cmd
}

// RepairIndexOptions collects all options for the repair index command.
type RepairIndexOptions struct {
	ReadAllPacks bool
}

func (opts *RepairIndexOptions) AddFlags(f *pflag.FlagSet) {
	f.BoolVar(&opts.ReadAllPacks, "read-all-packs", false, "read all pack files to generate new index from scratch")
}

func newRebuildIndexCommand(globalOptions *GlobalOptions) *cobra.Command {
	var opts RepairIndexOptions

	replacement := newRepairIndexCommand(globalOptions)
	cmd := &cobra.Command{
		Use:               "rebuild-index [flags]",
		Short:             replacement.Short,
		Long:              replacement.Long,
		Deprecated:        `Use "repair index" instead`,
		DisableAutoGenTag: true,
		// must create a new instance of the run function as it captures opts
		// by reference
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRebuildIndex(cmd.Context(), opts, *globalOptions, globalOptions.term)
		},
	}

	opts.AddFlags(cmd.Flags())
	return cmd
}

func runRebuildIndex(ctx context.Context, opts RepairIndexOptions, gopts GlobalOptions, term ui.Terminal) error {
	printer := ui.NewProgressPrinter(false, gopts.verbosity, term)

	ctx, repo, unlock, err := openWithExclusiveLock(ctx, gopts, false, printer)
	if err != nil {
		return err
	}
	defer unlock()

	err = repository.RepairIndex(ctx, repo, repository.RepairIndexOptions{
		ReadAllPacks: opts.ReadAllPacks,
	}, printer)
	if err != nil {
		return err
	}

	printer.P("done\n")
	return nil
}
