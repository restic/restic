package main

import (
	"github.com/restic/restic/internal/global"
	"github.com/spf13/cobra"
)

func newRepairCommand(globalOptions *global.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "repair",
		Short:             "Repair the repository",
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
