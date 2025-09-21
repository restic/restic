package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/restic/restic/internal/terminal"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"
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
	} else if !json && !terminal.StdoutCanUpdateStatus() || !show {
		interval = 0
	}
	return interval
}

// newTerminalProgressMax returns a progress.Counter that prints to terminal if provided.
func newTerminalProgressMax(show bool, max uint64, description string, term ui.Terminal) *progress.Counter {
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

		if final {
			term.SetStatus(nil)
			term.Print(status)
		} else {
			term.SetStatus([]string{status})
		}
	})
}

type terminalProgressPrinter struct {
	term ui.Terminal
	ui.Message
	show bool
}

func (t *terminalProgressPrinter) NewCounter(description string) *progress.Counter {
	return newTerminalProgressMax(t.show, 0, description, t.term)
}

func (t *terminalProgressPrinter) NewCounterTerminalOnly(description string) *progress.Counter {
	return newTerminalProgressMax(t.show && terminal.StdoutIsTerminal(), 0, description, t.term)
}

func newTerminalProgressPrinter(json bool, verbosity uint, term ui.Terminal) progress.Printer {
	if json {
		verbosity = 0
	}
	return &terminalProgressPrinter{
		term:    term,
		Message: *ui.NewMessage(term, verbosity),
		show:    verbosity > 0,
	}
}

func newIndexTerminalProgress(printer progress.Printer) *progress.Counter {
	return printer.NewCounterTerminalOnly("index files loaded")
}
