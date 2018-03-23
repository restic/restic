package main

import (
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
`,
	DisableAutoGenTag: true,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("restic %s compiled with %v on %v/%v\n",
			version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	},
}

func init() {
	cmdRoot.AddCommand(versionCmd)
}
