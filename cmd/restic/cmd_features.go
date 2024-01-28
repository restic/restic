package main

import (
	"fmt"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/feature"
	"github.com/restic/restic/internal/ui/table"

	"github.com/spf13/cobra"
)

// FIXME explain semantics

var featuresCmd = &cobra.Command{
	Use:   "features",
	Short: "Print list of feature flags",
	Long: `
The "features" command prints a list of supported feature flags.

EXIT STATUS
===========

Exit status is 0 if the command was successful, and non-zero if there was any error.
`,
	Hidden:            true,
	DisableAutoGenTag: true,
	RunE: func(_ *cobra.Command, args []string) error {
		if len(args) != 0 {
			return errors.Fatal("the feature command expects no arguments")
		}

		fmt.Printf("All Feature Flags:\n")
		flags := feature.Flag.List()

		tab := table.New()
		tab.AddColumn("Name", "{{ .Name }}")
		tab.AddColumn("Type", "{{ .Type }}")
		tab.AddColumn("Default", "{{ .Default }}")
		tab.AddColumn("Description", "{{ .Description }}")

		for _, flag := range flags {
			tab.AddRow(flag)
		}
		return tab.Write(globalOptions.stdout)
	},
}

func init() {
	cmdRoot.AddCommand(featuresCmd)
}
