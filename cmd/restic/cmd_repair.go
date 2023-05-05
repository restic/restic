package main

import (
	"github.com/spf13/cobra"
)

var cmdRepair = &cobra.Command{
	Use:   "repair",
	Short: "Repair the repository",
}

func init() {
	cmdRoot.AddCommand(cmdRepair)
}
