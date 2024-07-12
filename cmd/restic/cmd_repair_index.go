package main

import (
	"context"

	"github.com/restic/restic/internal/repository"
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

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
Exit status is 10 if the repository does not exist.
Exit status is 11 if the repository is already locked.
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

	err = repository.RepairIndex(ctx, repo, repository.RepairIndexOptions{
		ReadAllPacks: opts.ReadAllPacks,
	}, printer)
	if err != nil {
		return err
	}

	printer.P("done\n")
	return nil
}
