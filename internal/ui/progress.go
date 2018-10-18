package ui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strconv"
	"text/template"
	"time"

	"github.com/restic/restic/internal/ui/termstatus"
)

const (
	etaDONE = 0  // XXX how to represent "done"?
	etaNA   = -1 // XXX how to represent "unknown"?
)

// High level idea of what I am trying to do
//
// Each long-running command is modeled as a sequence of independent phases (better name is welcome),
// which happen strictly one after the another, i.e. without overlap in time. Performance
// of one phase cannot be used to estimate performance of other phases (hence "independent").
// For example, network download speed cannot be used to estimate local disk read speed and vise versa.
//
// For each in-progress phase I want to show to the user:
// * the phase name
// * %% and ETA of completion (when possible to estimate)
// * one-line message about number of files/bytes/packs/etc processed so far and total (depends on the nature of the phase)
// * optionally, few lines of info about files/packs currently being processed (not convinced this is terrible useful)
//
// At the end of the phase I want to show to the user:
// * the time it took to complete the phase
// * total number files/bytes/packs/etc processed during the phase (depends on the nature of the phase)
// * speed attained (if makes sense)

// ProgressUI provides periodic updates about long a running command.
type ProgressUI interface {
	E(msg string, args ...interface{})
	P(msg string, args ...interface{})
	V(msg string, args ...interface{})
	VV(msg string, args ...interface{})

	// TODO rename to StartPhase
	// Set currently running operation phase
	Set(title string, setup func(), metrics map[string]interface{}, progress, summary string)

	// Update executes op, then updates user-visible progress UI as necessary
	Update(op func())

	// TODO rename to FinishPhase
	Unset()
}

type progressPhase struct {
	title    string
	metrics  map[string]interface{}
	progress *template.Template
	summary  *template.Template
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

	// long-running command start time
	start time.Time

	phase progressPhase
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
				p.phase = progressPhase{}
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

func copyMetrics(original map[string]interface{}) map[string]interface{} {
	copy := make(map[string]interface{}, len(original))
	for k, v := range original {
		copy[k] = v
	}
	return copy
}

// Set currently running operation title, periodic progress and summary
// messages. all callbacks will be invoked from the "UI" thread
func (p *TermstatusProgressUI) Set(title string, setup func(), metrics map[string]interface{}, progress, summary string) {
	metrics = copyMetrics(metrics)
	metrics["stopwatch"] = StartStopwatch()
	// decorate progress messahe with running time, completion % and ETA, if available
	progress = "[{{.stopwatch.FormatDuration}}] " + progress
	if _, ok := metrics["percent"]; ok {
		progress = progress + " {{.percent.FormatPercent}} ETA {{.percent.FormatETA .stopwatch}}"
	}
	p.updates <- func() {
		p.diplaySummary() // display summary of the prior phase if any
		if setup != nil {
			setup()
		}
		p.phase = progressPhase{
			title:    title,
			metrics:  metrics, // XXX do I need to make a copy, just to be safe?
			progress: parseTemplate("progress", progress),
			summary:  parseTemplate("summary", summary),
		}
		p.displayProgress(true) // display initial progress
	}
}

func (p *TermstatusProgressUI) Unset() {
	p.updates <- func() {
		p.diplaySummary() // display summary of the prior phase if any
		p.phase = progressPhase{}
		p.displayProgress(false)
	}
}

// Finish stops UI updates and prints summary message.
func (p *TermstatusProgressUI) Finish() {
	close(p.updates)

	<-p.running // let the worker finish what it's doing
}

func (p *TermstatusProgressUI) diplaySummary() {
	if p.phase.summary != nil {
		p.P(executeTemplate(p.phase.summary, p.phase.metrics))
	}
}

func parseTemplate(name, text string) *template.Template {
	return template.Must(template.New(name).Parse(text))
}

func executeTemplate(t *template.Template, data interface{}) string {
	buf := new(bytes.Buffer)
	err := t.Execute(buf, data)
	if err != nil {
		panic(err)
	}
	return buf.String()
}

func (p *TermstatusProgressUI) displayProgress(first bool) {
	if p.phase.title == "" {
		if p.term.CanDisplayStatus() {
			p.term.SetStatus([]string{})
		}
		return
	}

	msg := executeTemplate(p.phase.progress, p.phase.metrics)

	if p.term.CanDisplayStatus() {
		// TODO consider single line to include title, progress and ETA
		// TODO asci-art progress bar, if completion percent is available
		var lines []string
		if p.phase.title != "" {
			lines = append(lines, p.phase.title)
		}
		lines = append(lines, msg)
		p.term.SetStatus(lines)
	} else {
		// on dumb terminals print title once, then progress message
		if first {
			p.P("%s", p.phase.title)
		} else {
			p.V("%s", msg)
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

func formatPercent(percent float64) string {
	switch {
	case percent < 0:
		percent = 0
	case percent > 1:
		percent = 1
	}
	return fmt.Sprintf("%3.2f%%", 100*percent)
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

func FormatDuration(d time.Duration) string {
	return formatDuration(d)
}

// FormatDurationSince returns time elapsed since t as HH:mm:ss string
func FormatDurationSince(t time.Time) string {
	return formatDuration(time.Since(t))
}

// FormatETA ETA string
func formatETA(elapsed time.Duration, percent float64) string {
	eta := eta(elapsed, percent)
	switch {
	case eta == etaDONE:
		return "DONE"
	case eta == etaNA:
		return "N/A"
	default:
		return formatDuration(eta)
	}
}

func eta(elapsed time.Duration, percent float64) time.Duration {
	// XXX original inputs are integers, don't like float64 here at all
	switch {
	case percent >= 1:
		return etaDONE
	case percent < 0:
		return etaNA
	}
	return time.Duration(float64(elapsed) / percent)
}
