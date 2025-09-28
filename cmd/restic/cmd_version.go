package main

import (
	"encoding/json"
	"runtime"

	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/ui"
	"github.com/spf13/cobra"
)

func newVersionCommand(globalOptions *global.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long: `
The "version" command prints detailed information about the build environment
and the version of this software.

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
`,
		DisableAutoGenTag: true,
		Run: func(_ *cobra.Command, _ []string) {
			printer := ui.NewProgressPrinter(globalOptions.JSON, globalOptions.Verbosity, globalOptions.Term)

			if globalOptions.JSON {
				type jsonVersion struct {
					MessageType string `json:"message_type"` // version
					Version     string `json:"version"`
					GoVersion   string `json:"go_version"`
					GoOS        string `json:"go_os"`
					GoArch      string `json:"go_arch"`
				}

				jsonS := jsonVersion{
					MessageType: "version",
					Version:     global.Version,
					GoVersion:   runtime.Version(),
					GoOS:        runtime.GOOS,
					GoArch:      runtime.GOARCH,
				}

				err := json.NewEncoder(globalOptions.Term.OutputWriter()).Encode(jsonS)
				if err != nil {
					printer.E("JSON encode failed: %v\n", err)
					return
				}
			} else {
				printer.S("restic %s compiled with %v on %v/%v\n",
					global.Version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
			}
		},
	}
	return cmd
}
