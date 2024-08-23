package main

import (
	"github.com/spf13/cobra"
)

var cmdRepair = &cobra.Command{
	Use:               "repair",
	Short:             "Repair the repository",
	GroupID:           cmdGroupDefault,
}

func init() {
	cmdRoot.AddCommand(cmdRepair)
}
