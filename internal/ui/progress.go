package ui

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/restic/restic/internal/ui/termstatus"
)

// ProgressUI provides periodic updates about long a running operation
type ProgressUI interface {
	E(msg string, args ...interface{})
	P(msg string, args ...interface{})
	V(msg string, args ...interface{})
	VV(msg string, args ...interface{})

	// Set currently running operation title, periodic progress and summary
	// messages.
	// update, progress and summary callback invocations are serialized
	Set(title string, progress, summary func() []string)

	// Update executes op, then displays user-visible progress message(s).
	// update, progress and summary callback invocations are serialized.
	Update(op func())
}

// TermstatusProgressUI implements ProgressUI using termstatus.Terminal.
// Supports both "fancy" (i.e. ansi) and dumb terminals. Respects
// global message verbosity level.
// Clients are expected to run  "UI" thread (Run function) in a separate
// goroutine.
// Clients are not expected to use "real" stdout/stderr, they should either
// use E/P/V/VV methods (recommended) or StdioWrapper's Stdout/Stderr.
type TermstatusProgressUI struct {
	*Message
	*StdioWrapper

	term    *termstatus.Terminal
	updates chan func()
	running chan struct{}

	minUpdatePause time.Duration

	title    string
	progress func() []string
	summary  func() []string

	start time.Time
}

var _ ProgressUI = &TermstatusProgressUI{}

// NewTermstatusProgressUI returns new termstatus-based ProgressUI instance
func NewTermstatusProgressUI(term *termstatus.Terminal, verbosity uint) *TermstatusProgressUI {
	r := &TermstatusProgressUI{
		Message:        NewMessage(term, verbosity),
		StdioWrapper:   NewStdioWrapper(term),
		term:           term,
		updates:        make(chan func(), 16), // TODO let clients choose desired concurrency
		running:        make(chan struct{}),
		start:          time.Now(),
		minUpdatePause: time.Second / 60, // limit to 60fps by default
	}

	if s, ok := os.LookupEnv("RESTIC_PROGRESS_FPS"); ok {
		fps, err := strconv.Atoi(s)
		if err == nil && fps >= 1 {
			if fps > 60 {
				fps = 60
			}
			r.minUpdatePause = time.Second / time.Duration(fps)
		}
	} else if !term.CanDisplayStatus() {
		r.minUpdatePause = time.Second * 10 // update every 10 seconds on dumb terminals
	}

	return r
}

// Run regularly updates the status lines. It should be called in a separate
// goroutine.
func (p *TermstatusProgressUI) Run(ctx context.Context) error {
	var lastUpdate time.Time

	t := time.NewTicker(time.Second)
	defer t.Stop()

forever:
	for {
		select {
		case <-ctx.Done():
			break forever
		case op, ok := <-p.updates:
			if !ok {
				p.term.SetStatus([]string{})
				p.diplaySummary()
				p.title = ""
				p.progress = nil
				p.summary = nil
				break forever
			}
			op()
		case <-t.C:
		}

		if time.Since(lastUpdate) >= p.minUpdatePause {
			p.displayProgress(false)
			lastUpdate = time.Now()
		}
	}

	close(p.running)

	return nil
}

// Update executes op on the '"UI" thread, then displays user-visible progress message(s).
func (p *TermstatusProgressUI) Update(op func()) {
	p.updates <- op
}

// Set currently running operation title, periodic progress and summary
// messages. progress and summary callbacks will be invoked from the "UI" thread
func (p *TermstatusProgressUI) Set(title string, progress, summary func() []string) {
	p.updates <- func() {
		p.diplaySummary()
		p.title = title
		p.progress = progress
		p.summary = summary
		p.displayProgress(true)
	}
}

// Finish stops UI updates and prints summary message.
func (p *TermstatusProgressUI) Finish() {
	close(p.updates)

	<-p.running // let the worker finish what it's doing
}

func (p *TermstatusProgressUI) diplaySummary() {
	if p.summary != nil {
		for _, line := range p.summary() {
			p.P(line)
		}
	}
}

func (p *TermstatusProgressUI) displayProgress(first bool) {
	duration := FormatDurationSince(p.start)

	progress := func(lines []string) []string {
		if p.progress != nil {
			for _, line := range p.progress() {
				lines = append(lines, fmt.Sprintf("[%s] %s", duration, line))
			}
		}
		return lines
	}

	if p.term.CanDisplayStatus() {
		var lines []string
		if p.title != "" {
			lines = append(lines, p.title)
		}
		lines = progress(lines)
		p.term.SetStatus(lines)
	} else {
		// on dumb terminals print title once, then progress message
		if first {
			p.P("%s", p.title)
		} else {
			lines := progress([]string{})
			if len(lines) > 0 {
				if len(lines) > 1 {
					// separate multiline messages so they are easier to read
					lines = append(lines, "\n")
				}
				for _, line := range lines {
					p.V("%s", line)
				}
			}
		}
	}

}

// FormatBytes formats provided number in best matching binary units (B/KiB/MiB/etc)
func FormatBytes(c uint64) string {
	b := float64(c)
	switch {
	case c > 1<<40:
		return fmt.Sprintf("%.3f TiB", b/(1<<40))
	case c > 1<<30:
		return fmt.Sprintf("%.3f GiB", b/(1<<30))
	case c > 1<<20:
		return fmt.Sprintf("%.3f MiB", b/(1<<20))
	case c > 1<<10:
		return fmt.Sprintf("%.3f KiB", b/(1<<10))
	default:
		return fmt.Sprintf("%d B", c)
	}
}

// FormatPercent returns provided numerator as 0..100 percetage of the provided
// denominator. Returns empty string if demoninator is 0. Returns 100 if numerator
// is larger than denominator.
func FormatPercent(numerator uint64, denominator uint64) string {
	if denominator == 0 {
		return ""
	}

	percent := 100.0 * float64(numerator) / float64(denominator)

	if percent > 100 {
		percent = 100
	}

	return fmt.Sprintf("%3.2f%%", percent)
}

// FormatSeconds returns provided number of as HH:mm:ss string
func FormatSeconds(sec uint64) string {
	hours := sec / 3600
	sec -= hours * 3600
	min := sec / 60
	sec -= min * 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, min, sec)
	}

	return fmt.Sprintf("%d:%02d", min, sec)
}

func formatDuration(d time.Duration) string {
	sec := uint64(d / time.Second)
	return FormatSeconds(sec)
}

// FormatDurationSince returns time elapsed since t as HH:mm:ss string
func FormatDurationSince(t time.Time) string {
	return formatDuration(time.Since(t))
}
