package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"
	"github.com/restic/restic/internal/ui/termstatus"
)

// calculateProgressInterval returns the interval configured via RESTIC_PROGRESS_FPS
// or if unset returns an interval for 60fps on interactive terminals and 0 (=disabled)
// for non-interactive terminals or when run using the --quiet flag
func calculateProgressInterval(show bool, json bool) time.Duration {
	interval := time.Second / 60
	fps, err := strconv.ParseFloat(os.Getenv("RESTIC_PROGRESS_FPS"), 64)
	if err == nil && fps > 0 {
		if fps > 60 {
			fps = 60
		}
		interval = time.Duration(float64(time.Second) / fps)
	} else if !json && !stdoutCanUpdateStatus() || !show {
		interval = 0
	}
	return interval
}

// newTerminalProgressMax returns a progress.Counter that prints to stdout or terminal if provided.
func newGenericProgressMax(show bool, max uint64, description string, print func(status string, final bool)) *progress.Counter {
	if !show {
		return nil
	}
	interval := calculateProgressInterval(show, false)

	return progress.NewCounter(interval, max, func(v uint64, max uint64, d time.Duration, final bool) {
		var status string
		if max == 0 {
			status = fmt.Sprintf("[%s]          %d %s",
				ui.FormatDuration(d), v, description)
		} else {
			status = fmt.Sprintf("[%s] %s  %d / %d %s",
				ui.FormatDuration(d), ui.FormatPercent(v, max), v, max, description)
		}

		print(status, final)
	})
}

func newTerminalProgressMax(show bool, max uint64, description string, term *termstatus.Terminal) *progress.Counter {
	return newGenericProgressMax(show, max, description, func(status string, final bool) {
		if final {
			term.SetStatus(nil)
			term.Print(status)
		} else {
			term.SetStatus([]string{status})
		}
	})
}

// newProgressMax calls newTerminalProgress without a terminal (print to stdout)
func newProgressMax(show bool, max uint64, description string) *progress.Counter {
	return newGenericProgressMax(show, max, description, printProgress)
}

func printProgress(status string, final bool) {

	canUpdateStatus := stdoutCanUpdateStatus()

	w := stdoutTerminalWidth()
	if w > 0 {
		if w < 3 {
			status = termstatus.Truncate(status, w)
		} else {
			trunc := termstatus.Truncate(status, w-3)
			if len(trunc) < len(status) {
				status = trunc + "..."
			}
		}
	}

	var carriageControl, clear string

	if canUpdateStatus {
		clear = clearLine(w)
	}

	if !(strings.HasSuffix(status, "\r") || strings.HasSuffix(status, "\n")) {
		if canUpdateStatus {
			carriageControl = "\r"
		} else {
			carriageControl = "\n"
		}
	}

	_, _ = os.Stdout.Write([]byte(clear + status + carriageControl))
	if final {
		_, _ = os.Stdout.Write([]byte("\n"))
	}
}

func newIndexProgress(quiet bool, json bool) *progress.Counter {
	return newProgressMax(!quiet && !json && stdoutIsTerminal(), 0, "index files loaded")
}

func newIndexTerminalProgress(quiet bool, json bool, term *termstatus.Terminal) *progress.Counter {
	return newTerminalProgressMax(!quiet && !json && stdoutIsTerminal(), 0, "index files loaded", term)
}

type terminalProgressPrinter struct {
	term *termstatus.Terminal
	ui.Message
	show bool
}

func (t *terminalProgressPrinter) NewCounter(description string) *progress.Counter {
	return newTerminalProgressMax(t.show, 0, description, t.term)
}

func newTerminalProgressPrinter(verbosity uint, term *termstatus.Terminal) progress.Printer {
	return &terminalProgressPrinter{
		term:    term,
		Message: *ui.NewMessage(term, verbosity),
		show:    verbosity > 0,
	}
}
