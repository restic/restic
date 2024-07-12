package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	godebug "runtime/debug"

	"github.com/spf13/cobra"
	"go.uber.org/automaxprocs/maxprocs"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/feature"
	"github.com/restic/restic/internal/options"
	"github.com/restic/restic/internal/restic"
)

func init() {
	// don't import `go.uber.org/automaxprocs` to disable the log output
	_, _ = maxprocs.Set()
}

var ErrOK = errors.New("ok")

// cmdRoot is the base command when no other command has been specified.
var cmdRoot = &cobra.Command{
	Use:   "restic",
	Short: "Backup and restore files",
	Long: `
restic is a backup program which allows saving multiple revisions of files and
directories in an encrypted repository stored on different backends.

The full documentation can be found at https://restic.readthedocs.io/ .
`,
	SilenceErrors:     true,
	SilenceUsage:      true,
	DisableAutoGenTag: true,

	PersistentPreRunE: func(c *cobra.Command, _ []string) error {
		// set verbosity, default is one
		globalOptions.verbosity = 1
		if globalOptions.Quiet && globalOptions.Verbose > 0 {
			return errors.Fatal("--quiet and --verbose cannot be specified at the same time")
		}

		switch {
		case globalOptions.Verbose >= 2:
			globalOptions.verbosity = 3
		case globalOptions.Verbose > 0:
			globalOptions.verbosity = 2
		case globalOptions.Quiet:
			globalOptions.verbosity = 0
		}

		// parse extended options
		opts, err := options.Parse(globalOptions.Options)
		if err != nil {
			return err
		}
		globalOptions.extended = opts
		if !needsPassword(c.Name()) {
			return nil
		}
		pwd, err := resolvePassword(globalOptions, "RESTIC_PASSWORD")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Resolving password failed: %v\n", err)
			Exit(1)
		}
		globalOptions.password = pwd

		// run the debug functions for all subcommands (if build tag "debug" is
		// enabled)
		return runDebug()
	},
	PersistentPostRun: func(_ *cobra.Command, _ []string) {
		stopDebug()
	},
}

// Distinguish commands that need the password from those that work without,
// so we don't run $RESTIC_PASSWORD_COMMAND for no reason (it might prompt the
// user for authentication).
func needsPassword(cmd string) bool {
	switch cmd {
	case "cache", "generate", "help", "options", "self-update", "version", "__complete":
		return false
	default:
		return true
	}
}

func tweakGoGC() {
	// lower GOGC from 100 to 50, unless it was manually overwritten by the user
	oldValue := godebug.SetGCPercent(50)
	if oldValue != 100 {
		godebug.SetGCPercent(oldValue)
	}
}

func main() {
	tweakGoGC()
	// install custom global logger into a buffer, if an error occurs
	// we can show the logs
	logBuffer := bytes.NewBuffer(nil)
	log.SetOutput(logBuffer)

	err := feature.Flag.Apply(os.Getenv("RESTIC_FEATURES"), func(s string) {
		fmt.Fprintln(os.Stderr, s)
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		Exit(1)
	}

	debug.Log("main %#v", os.Args)
	debug.Log("restic %s compiled with %v on %v/%v",
		version, runtime.Version(), runtime.GOOS, runtime.GOARCH)

	ctx := createGlobalContext()
	err = cmdRoot.ExecuteContext(ctx)

	if err == nil {
		err = ctx.Err()
	} else if err == ErrOK {
		// ErrOK overwrites context cancelation errors
		err = nil
	}

	switch {
	case restic.IsAlreadyLocked(err):
		fmt.Fprintf(os.Stderr, "%v\nthe `unlock` command can be used to remove stale locks\n", err)
	case err == ErrInvalidSourceData:
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	case errors.IsFatal(err):
		fmt.Fprintf(os.Stderr, "%v\n", err)
	case err != nil:
		fmt.Fprintf(os.Stderr, "%+v\n", err)

		if logBuffer.Len() > 0 {
			fmt.Fprintf(os.Stderr, "also, the following messages were logged by a library:\n")
			sc := bufio.NewScanner(logBuffer)
			for sc.Scan() {
				fmt.Fprintln(os.Stderr, sc.Text())
			}
		}
	}

	var exitCode int
	switch {
	case err == nil:
		exitCode = 0
	case err == ErrInvalidSourceData:
		exitCode = 3
	case errors.Is(err, ErrNoRepository):
		exitCode = 10
	case restic.IsAlreadyLocked(err):
		exitCode = 11
	case errors.Is(err, context.Canceled):
		exitCode = 130
	default:
		exitCode = 1
	}
	Exit(exitCode)
}
