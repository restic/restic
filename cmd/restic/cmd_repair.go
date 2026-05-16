package main

import (
	"github.com/restic/restic/internal/global"
	"github.com/spf13/cobra"
)

func newRepairCommand(globalOptions *global.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repair",
		Short: "Repair the repository",
		Long: `
The "repair" command repairs damaged repositories. It provides subcommands to
rebuild the index, salvage damaged pack files, and repair broken snapshots.
`,
		GroupID:           cmdGroupDefault,
		DisableAutoGenTag: true,
	}

	cmd.AddCommand(
		newRepairIndexCommand(globalOptions),
		newRepairPacksCommand(globalOptions),
		newRepairSnapshotsCommand(globalOptions),
	)
	return cmd
}
