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
	interval := time.Second / 10
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
	v    uint
}

func (t *progressPrinter) NewCounter(description string) *progress.Counter {
	return newProgressMax(t.v > 0, 0, description, t.term)
}

func (t *progressPrinter) NewCounterTerminalOnly(description string) *progress.Counter {
	return newProgressMax(t.v > 0 && t.term.OutputIsTerminal(), 0, description, t.term)
}

func (t *progressPrinter) E(msg string, args ...interface{}) {
	t.term.Error(fmt.Sprintf(msg, args...))
}

func (t *progressPrinter) S(msg string, args ...interface{}) {
	t.term.Print(fmt.Sprintf(msg, args...))
}

func (t *progressPrinter) PT(msg string, args ...interface{}) {
	if t.term.OutputIsTerminal() && t.v >= 1 {
		t.term.Print(fmt.Sprintf(msg, args...))
	}
}

func (t *progressPrinter) P(msg string, args ...interface{}) {
	if t.v >= 1 {
		t.term.Print(fmt.Sprintf(msg, args...))
	}
}

func (t *progressPrinter) V(msg string, args ...interface{}) {
	if t.v >= 2 {
		t.term.Print(fmt.Sprintf(msg, args...))
	}
}

func (t *progressPrinter) VV(msg string, args ...interface{}) {
	if t.v >= 3 {
		t.term.Print(fmt.Sprintf(msg, args...))
	}
}

func NewProgressPrinter(json bool, verbosity uint, term Terminal) progress.Printer {
	if json {
		verbosity = 0
	}
	return &progressPrinter{
		term: term,
		v:    verbosity,
	}
}
