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
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/automaxprocs/maxprocs"

	"github.com/restic/restic/internal/backend/all"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/feature"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/tracing"
	"github.com/restic/restic/internal/ui/termstatus"
)

func init() {
	// don't import `go.uber.org/automaxprocs` to disable the log output
	_, _ = maxprocs.Set()
}

var ErrOK = errors.New("ok")

var cmdGroupDefault = "default"
var cmdGroupAdvanced = "advanced"

func newRootCommand(globalOptions *global.Options) *cobra.Command {
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

		PersistentPreRunE: func(c *cobra.Command, args []string) error {
			switch c.Name() {
			case "__complete", "__completeNoDesc":
				return nil
			}
			if globalOptions.TraceURL != "" {
				shutdown, err := tracing.Setup(c.Context(), globalOptions.TraceURL, globalOptions.TraceService)
				if err != nil {
					// Non-fatal: warn and continue without tracing.
					_, _ = fmt.Fprintf(os.Stderr, "warning: tracing setup failed: %v\n", err)
				} else {
					ctx := tracing.ExtractParentContext(c.Context(), globalOptions.TraceParentID)
					ctx = startCommandTrace(ctx, c, args, globalOptions, shutdown)
					c.SetContext(ctx)
				}
			}
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
	global.RegisterProfiling(cmd, os.Stderr)

	return cmd
}

// startCommandTrace creates the process-ancestry span chain and the command
// span, wires them into ctx, and stores a cleanup function in gopts that ends
// all spans and shuts down the exporter. It returns the updated context.
func startCommandTrace(
	ctx context.Context,
	c *cobra.Command,
	_ []string,
	gopts *global.Options,
	shutdown func(context.Context) error,
) context.Context {
	sysInfo := tracing.Collect()
	t := tracing.Tracer()

	// Build a chain of spans for each ancestor process, oldest first.
	// Each span is a child of the previous one so the process tree is visible.
	var ancestorSpans []trace.Span
	for _, proc := range sysInfo.Ancestry {
		name := proc.Comm
		if name == "" {
			name = fmt.Sprintf("pid-%d", proc.PID)
		}
		var span trace.Span
		ctx, span = t.Start(ctx, name, trace.WithSpanKind(trace.SpanKindInternal))
		span.SetAttributes(
			attribute.Int("process.pid", proc.PID),
			attribute.Int("process.ppid", proc.PPID),
			attribute.String("process.command_line", proc.CmdLine),
		)
		ancestorSpans = append(ancestorSpans, span)
	}

	// The restic command span is a child of the innermost ancestor span
	// (or of the external parent supplied via --trace-id-parent).
	ctx, cmdSpan := t.Start(ctx, "restic."+c.Name(), trace.WithSpanKind(trace.SpanKindClient))
	cmdSpan.SetAttributes(
		attribute.StringSlice("process.command_args", os.Args),
		attribute.String("enduser.id", sysInfo.User),
		attribute.String("enduser.uid", sysInfo.UserID),
		attribute.String("host.name", sysInfo.FQDN),
		attribute.String("restic.command", c.Name()),
	)
	// Capture flag names and values for observability.
	var flagPairs []string
	c.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Changed {
			flagPairs = append(flagPairs, f.Name+"="+f.Value.String())
		}
	})
	if len(flagPairs) > 0 {
		cmdSpan.SetAttributes(attribute.String("restic.flags", strings.Join(flagPairs, " ")))
	}

	gopts.TraceCleanup = func(cleanupCtx context.Context, cmdErr error) {
		if cmdErr != nil {
			cmdSpan.RecordError(cmdErr)
			cmdSpan.SetStatus(codes.Error, cmdErr.Error())
		} else {
			cmdSpan.SetStatus(codes.Ok, "")
		}
		cmdSpan.End()
		// End ancestor spans in reverse order (innermost first).
		for i := len(ancestorSpans) - 1; i >= 0; i-- {
			ancestorSpans[i].End()
		}
		_ = shutdown(cleanupCtx)
	}

	return ctx
}

// Distinguish commands that need the password from those that work without,
// so we don't run $RESTIC_PASSWORD_COMMAND for no reason (it might prompt the
// user for authentication).
func needsPassword(cmd string) bool {
	switch cmd {
	case "cache", "generate", "help", "options", "self-update", "version", "__complete", "__completeNoDesc":
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

func printExitError(globalOptions global.Options, code int, message string) {
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
		global.Version, runtime.Version(), runtime.GOOS, runtime.GOARCH)

	globalOptions := global.Options{
		Backends: all.Backends(),
	}
	func() {
		term, cancel := termstatus.Setup(os.Stdin, os.Stdout, os.Stderr, globalOptions.Quiet)
		defer cancel()
		globalOptions.Term = term
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

	if globalOptions.TraceCleanup != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		globalOptions.TraceCleanup(cleanupCtx, err)
	}

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
			exitMessage += " also, the following messages were logged by a library:\n"
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
	case errors.Is(err, global.ErrNoRepository):
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
