package main

import (
	"github.com/spf13/cobra"
)

func newKeyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "key",
		Short: "Manage keys (passwords)",
		Long: `
The "key" command allows you to set multiple access keys or passwords
per repository.
	`,
		DisableAutoGenTag: true,
		GroupID:           cmdGroupDefault,
	}

	cmd.AddCommand(
		newKeyAddCommand(),
		newKeyListCommand(),
		newKeyPasswdCommand(),
		newKeyRemoveCommand(),
	)
	return cmd
}
