package main

import (
	"fmt"
	"os"
	"restic"
	"restic/debug"
	"runtime"

	"github.com/spf13/cobra"

	"restic/errors"
)

// cmdRoot is the base command when no other command has been specified.
var cmdRoot = &cobra.Command{
	Use:   "restic",
	Short: "backup and restore files",
	Long: `
restic is a backup program which allows saving multiple revisions of files and
directories in an encrypted repository stored on different backends.
`,
	SilenceErrors:    true,
	SilenceUsage:     true,
	PersistentPreRun: parseEnvironment,
}

func init() {
	// set GOMAXPROCS to number of CPUs
	if runtime.Version() < "go1.5" {
		gomaxprocs := os.Getenv("GOMAXPROCS")
		debug.Log("read GOMAXPROCS from env variable, value: %s", gomaxprocs)
		if gomaxprocs == "" {
			runtime.GOMAXPROCS(runtime.NumCPU())
		}
	}
}

func main() {
	debug.Log("main %#v", os.Args)
	err := cmdRoot.Execute()

	switch {
	case restic.IsAlreadyLocked(errors.Cause(err)):
		fmt.Fprintf(os.Stderr, "%v\nthe `unlock` command can be used to remove stale locks\n", err)
	case errors.IsFatal(errors.Cause(err)):
		fmt.Fprintf(os.Stderr, "%v\n", err)
	case err != nil:
		fmt.Fprintf(os.Stderr, "%+v\n", err)
	}

	RunCleanupHandlers()

	if err != nil {
		os.Exit(1)
	}
}
