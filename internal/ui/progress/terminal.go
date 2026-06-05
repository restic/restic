package progress

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/restic/restic/internal/ui"
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
func newProgressMax(show bool, max uint64, description string, term ui.Terminal) *Counter {
	if !show {
		return nil
	}
	interval := CalculateProgressInterval(show, false, term.CanUpdateStatus())

	return NewCounter(interval, max, func(v uint64, max uint64, d time.Duration, final bool) {
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

type terminalPrinter struct {
	term ui.Terminal
	v    uint
}

func (t *terminalPrinter) NewCounter(description string) *Counter {
	return newProgressMax(t.v > 0, 0, description, t.term)
}

func (t *terminalPrinter) NewCounterTerminalOnly(description string) *Counter {
	return newProgressMax(t.v > 0 && t.term.OutputIsTerminal(), 0, description, t.term)
}

func (t *terminalPrinter) E(msg string, args ...interface{}) {
	t.term.Error(fmt.Sprintf(msg, args...))
}

func (t *terminalPrinter) S(msg string, args ...interface{}) {
	t.term.Print(fmt.Sprintf(msg, args...))
}

func (t *terminalPrinter) PT(msg string, args ...interface{}) {
	if t.term.OutputIsTerminal() && t.v >= 1 {
		t.term.Print(fmt.Sprintf(msg, args...))
	}
}

func (t *terminalPrinter) P(msg string, args ...interface{}) {
	if t.v >= 1 {
		t.term.Print(fmt.Sprintf(msg, args...))
	}
}

func (t *terminalPrinter) V(msg string, args ...interface{}) {
	if t.v >= 2 {
		t.term.Print(fmt.Sprintf(msg, args...))
	}
}

func (t *terminalPrinter) VV(msg string, args ...interface{}) {
	if t.v >= 3 {
		t.term.Print(fmt.Sprintf(msg, args...))
	}
}

func NewTerminalPrinter(json bool, verbosity uint, term ui.Terminal) Printer {
	if json {
		verbosity = 0
	}
	return &terminalPrinter{
		term: term,
		v:    verbosity,
	}
}
