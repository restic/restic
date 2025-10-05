package ui

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/restic/restic/internal/ui/progress"
)

// CalculateProgressInterval returns the interval configured via RESTIC_PROGRESS_FPS
// or if unset returns an interval for 60fps on interactive terminals and 0 (=disabled)
// for non-interactive terminals or when run using the --quiet flag
func CalculateProgressInterval(show bool, json bool, canUpdateStatus bool) time.Duration {
	interval := time.Second / 60
	fps, err := strconv.ParseFloat(os.Getenv("RESTIC_PROGRESS_FPS"), 64)
	if err == nil && fps > 0 {
		if fps > 60 {
			fps = 60
		}
		interval = time.Duration(float64(time.Second) / fps)
	} else if !json && !canUpdateStatus || !show {
		interval = 0
	}
	return interval
}

// newProgressMax returns a progress.Counter that prints to terminal if provided.
func newProgressMax(show bool, max uint64, description string, term Terminal) *progress.Counter {
	if !show {
		return nil
	}
	interval := CalculateProgressInterval(show, false, term.CanUpdateStatus())

	return progress.NewCounter(interval, max, func(v uint64, max uint64, d time.Duration, final bool) {
		var status string
		if max == 0 {
			status = fmt.Sprintf("[%s]          %d %s",
				FormatDuration(d), v, description)
		} else {
			status = fmt.Sprintf("[%s] %s  %d / %d %s",
				FormatDuration(d), FormatPercent(v, max), v, max, description)
		}

		if final {
			term.SetStatus(nil)
			term.Print(status)
		} else {
			term.SetStatus([]string{status})
		}
	})
}

type progressPrinter struct {
	term Terminal
	Message
	show bool
}

func (t *progressPrinter) NewCounter(description string) *progress.Counter {
	return newProgressMax(t.show, 0, description, t.term)
}

func (t *progressPrinter) NewCounterTerminalOnly(description string) *progress.Counter {
	return newProgressMax(t.show && t.term.OutputIsTerminal(), 0, description, t.term)
}

func NewProgressPrinter(json bool, verbosity uint, term Terminal) progress.Printer {
	if json {
		verbosity = 0
	}
	return &progressPrinter{
		term:    term,
		Message: *NewMessage(term, verbosity),
		show:    verbosity > 0,
	}
}
