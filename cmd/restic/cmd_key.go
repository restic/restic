package main

import (
	"github.com/spf13/cobra"
)

var cmdKey = &cobra.Command{
	Use:   "key",
	Short: "Manage keys (passwords)",
	Long: `
The "key" command allows you to set multiple access keys or passwords
per repository.
	`,
}

func init() {
	cmdRoot.AddCommand(cmdKey)
}
