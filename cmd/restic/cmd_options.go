package main

import (
	"fmt"

	"github.com/restic/restic/internal/options"

	"github.com/spf13/cobra"
)

var optionsCmd = &cobra.Command{
	Use:   "options",
	Short: "Print list of extended options",
	Long: `
The "options" command prints a list of extended options.

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
`,
	Hidden:            true,
	DisableAutoGenTag: true,
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Printf("All Extended Options:\n")
		var maxLen int
		for _, opt := range options.List() {
			if l := len(opt.Namespace + "." + opt.Name); l > maxLen {
				maxLen = l
			}
		}
		for _, opt := range options.List() {
			fmt.Printf("  %*s  %s\n", -maxLen, opt.Namespace+"."+opt.Name, opt.Text)
		}
	},
}

func init() {
	cmdRoot.AddCommand(optionsCmd)
}
