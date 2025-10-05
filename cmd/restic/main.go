package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/termstatus"
)

func init() {
	// don't import `go.uber.org/automaxprocs` to disable the log output
	_, _ = maxprocs.Set()
}

var ErrOK = errors.New("ok")

var cmdGroupDefault = "default"
var cmdGroupAdvanced = "advanced"

func newRootCommand(globalOptions *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
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
			return globalOptions.PreRun(needsPassword(c.Name()))
		},
	}

	cmd.AddGroup(
		&cobra.Group{
			ID:    cmdGroupDefault,
			Title: "Available Commands:",
		},
		&cobra.Group{
			ID:    cmdGroupAdvanced,
			Title: "Advanced Options:",
		},
	)

	globalOptions.AddFlags(cmd.PersistentFlags())

	// Use our "generate" command instead of the cobra provided "completion" command
	cmd.CompletionOptions.DisableDefaultCmd = true

	// globalOptions is passed to commands by reference to allow PersistentPreRunE to modify it
	cmd.AddCommand(
		newBackupCommand(globalOptions),
		newCacheCommand(globalOptions),
		newCatCommand(globalOptions),
		newCheckCommand(globalOptions),
		newCopyCommand(globalOptions),
		newDiffCommand(globalOptions),
		newDumpCommand(globalOptions),
		newFeaturesCommand(globalOptions),
		newFindCommand(globalOptions),
		newForgetCommand(globalOptions),
		newGenerateCommand(globalOptions),
		newInitCommand(globalOptions),
		newKeyCommand(globalOptions),
		newListCommand(globalOptions),
		newLsCommand(globalOptions),
		newMigrateCommand(globalOptions),
		newOptionsCommand(globalOptions),
		newPruneCommand(globalOptions),
		newRebuildIndexCommand(globalOptions),
		newRecoverCommand(globalOptions),
		newRepairCommand(globalOptions),
		newRestoreCommand(globalOptions),
		newRewriteCommand(globalOptions),
		newSnapshotsCommand(globalOptions),
		newStatsCommand(globalOptions),
		newTagCommand(globalOptions),
		newUnlockCommand(globalOptions),
		newVersionCommand(globalOptions),
	)

	registerDebugCommand(cmd, globalOptions)
	registerMountCommand(cmd, globalOptions)
	registerSelfUpdateCommand(cmd, globalOptions)
	registerProfiling(cmd, os.Stderr)

	return cmd
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

func printExitError(globalOptions GlobalOptions, code int, message string) {
	if globalOptions.JSON {
		type jsonExitError struct {
			MessageType string `json:"message_type"` // exit_error
			Code        int    `json:"code"`
			Message     string `json:"message"`
		}

		jsonS := jsonExitError{
			MessageType: "exit_error",
			Code:        code,
			Message:     message,
		}

		err := json.NewEncoder(os.Stderr).Encode(jsonS)
		if err != nil {
			// ignore error as there's no good way to handle it
			_, _ = fmt.Fprintf(os.Stderr, "JSON encode failed: %v\n", err)
			debug.Log("JSON encode failed: %v\n", err)
			return
		}
	} else {
		_, _ = fmt.Fprintf(os.Stderr, "%v\n", message)
	}
}

func main() {
	tweakGoGC()
	// install custom global logger into a buffer, if an error occurs
	// we can show the logs
	logBuffer := bytes.NewBuffer(nil)
	log.SetOutput(logBuffer)

	err := feature.Flag.Apply(os.Getenv("RESTIC_FEATURES"), func(s string) {
		_, _ = fmt.Fprintln(os.Stderr, s)
	})
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		Exit(1)
	}

	debug.Log("main %#v", os.Args)
	debug.Log("restic %s compiled with %v on %v/%v",
		version, runtime.Version(), runtime.GOOS, runtime.GOARCH)

	globalOptions := GlobalOptions{
		backends: collectBackends(),
	}
	func() {
		term, cancel := termstatus.Setup(os.Stdin, os.Stdout, os.Stderr, globalOptions.Quiet)
		defer cancel()
		globalOptions.term = term
		ctx := createGlobalContext(os.Stderr)
		err = newRootCommand(&globalOptions).ExecuteContext(ctx)
		switch err {
		case nil:
			err = ctx.Err()
		case ErrOK:
			// ErrOK overwrites context cancellation errors
			err = nil
		}
	}()

	var exitMessage string
	switch {
	case restic.IsAlreadyLocked(err):
		exitMessage = fmt.Sprintf("%v\nthe `unlock` command can be used to remove stale locks", err)
	case err == ErrInvalidSourceData:
		exitMessage = fmt.Sprintf("Warning: %v", err)
	case errors.IsFatal(err):
		exitMessage = err.Error()
	case errors.Is(err, repository.ErrNoKeyFound):
		exitMessage = fmt.Sprintf("Fatal: %v", err)
	case err != nil:
		exitMessage = fmt.Sprintf("%+v", err)

		if logBuffer.Len() > 0 {
			exitMessage += "also, the following messages were logged by a library:\n"
			sc := bufio.NewScanner(logBuffer)
			for sc.Scan() {
				exitMessage += fmt.Sprintln(sc.Text())
			}
		}
	}

	var exitCode int
	switch {
	case err == nil:
		exitCode = 0
	case err == ErrInvalidSourceData:
		exitCode = 3
	case errors.Is(err, ErrFailedToRemoveOneOrMoreSnapshots):
		exitCode = 3
	case errors.Is(err, ErrNoRepository):
		exitCode = 10
	case restic.IsAlreadyLocked(err):
		exitCode = 11
	case errors.Is(err, repository.ErrNoKeyFound):
		exitCode = 12
	case errors.Is(err, context.Canceled):
		exitCode = 130
	default:
		exitCode = 1
	}

	if exitCode != 0 {
		printExitError(globalOptions, exitCode, exitMessage)
	}
	Exit(exitCode)
}
