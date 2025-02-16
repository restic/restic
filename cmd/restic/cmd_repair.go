package main

import (
	"github.com/spf13/cobra"
)

func newRepairCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "repair",
		Short:             "Repair the repository",
		GroupID:           cmdGroupDefault,
		DisableAutoGenTag: true,
	}

	cmd.AddCommand(
		newRepairIndexCommand(),
		newRepairPacksCommand(),
		newRepairSnapshotsCommand(),
	)
	return cmd
}
