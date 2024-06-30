package main

import (
	"fmt"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/feature"
	"github.com/restic/restic/internal/ui/table"

	"github.com/spf13/cobra"
)

var featuresCmd = &cobra.Command{
	Use:   "features",
	Short: "Print list of feature flags",
	Long: `
The "features" command prints a list of supported feature flags.

To pass feature flags to restic, set the RESTIC_FEATURES environment variable
to "featureA=true,featureB=false". Specifying an unknown feature flag is an error.

A feature can either be in alpha, beta, stable or deprecated state.
An _alpha_ feature is disabled by default and may change in arbitrary ways between restic versions or be removed.
A _beta_ feature is enabled by default, but still can change in minor ways or be removed.
A _stable_ feature is always enabled and cannot be disabled. The flag will be removed in a future restic version.
A _deprecated_ feature is always disabled and cannot be enabled. The flag will be removed in a future restic version.

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
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
