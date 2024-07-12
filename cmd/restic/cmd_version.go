package main

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
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
		if globalOptions.JSON {
			type jsonVersion struct {
				Version   string `json:"version"`
				GoVersion string `json:"go_version"`
				GoOS      string `json:"go_os"`
				GoArch    string `json:"go_arch"`
			}

			jsonS := jsonVersion{
				Version:   version,
				GoVersion: runtime.Version(),
				GoOS:      runtime.GOOS,
				GoArch:    runtime.GOARCH,
			}

			err := json.NewEncoder(globalOptions.stdout).Encode(jsonS)
			if err != nil {
				Warnf("JSON encode failed: %v\n", err)
				return
			}
		} else {
			fmt.Printf("restic %s compiled with %v on %v/%v\n",
				version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
		}

	},
}

func init() {
	cmdRoot.AddCommand(versionCmd)
}
