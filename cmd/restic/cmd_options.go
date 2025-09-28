package main

import (
	"fmt"

	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/options"

	"github.com/spf13/cobra"
)

func newOptionsCommand(globalOptions *global.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "options",
		Short: "Print list of extended options",
		Long: `
The "options" command prints a list of extended options.

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
`,
		GroupID:           cmdGroupAdvanced,
		DisableAutoGenTag: true,
		Run: func(_ *cobra.Command, _ []string) {
			globalOptions.Term.Print("All Extended Options:")
			var maxLen int
			for _, opt := range options.List() {
				if l := len(opt.Namespace + "." + opt.Name); l > maxLen {
					maxLen = l
				}
			}
			for _, opt := range options.List() {
				globalOptions.Term.Print(fmt.Sprintf("  %*s  %s", -maxLen, opt.Namespace+"."+opt.Name, opt.Text))
			}
		},
	}
	return cmd
}
