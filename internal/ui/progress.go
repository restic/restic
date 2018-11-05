package ui

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/restic/restic/internal/ui/termstatus"
)

const (
	etaDONE = 0
	etaNA   = -1
)

// ProgressUI provides periodic updates about progress of a long running command.
//
// Long-running commands are modeled as sequences of independent phases,
// which happen strictly one after the another, i.e. without overlap in time. Performance
// of one phase cannot be used to estimate performance of other phases (hence "independent").
// For example, network download speed cannot be used to estimate local disk read speed and vise versa.
//
// ProgressUI renders the following messages for each long running command phase
// * transient phase progress information, which is cleared after phase finishes
//   - one-line progress so far, decorated with elapsed time and ETA if available
//   - optional multi-line status
// * phase summary, one or more lines with totals, running time, speed, etc, which is
//   printed after phase finishes
//
// ProgressUI clients are not expected to use "real" stdout/stderr, they should
// use ProgressUI E/P/V/VV methods.
type ProgressUI interface {
	E(msg string, args ...interface{})
	P(msg string, args ...interface{})
	V(msg string, args ...interface{})
	VV(msg string, args ...interface{})

	// StartPhase starts periodic progress report for a long running command phase.
	StartPhase(progress func() string, status func() []string, percent func() (int64, int64), summary func(time.Duration))

	// Update executes op, then updates user-visible progress UI as necessary
	Update(op func())

	// FinishPhase stops periodic progress report for the current phase and prints phase execution summary.
	FinishPhase()
}

type progressPhase struct {
	stopwatch stopwatch
	progress  func() string
	status    func() []string
	percent   func() (int64, int64)
	summary   func(time.Duration)
}

// TermstatusProgressUI implements ProgressUI using termstatus.Terminal.
// Supports both "fancy" (i.e. ansi) and dumb terminals. Respects
// global message verbosity level.
// Clients are expected to run  "UI" thread (Run function) in a separate
// goroutine.
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

// StartPhase starts periodic progress report for a long running command phase.
// Provided callback functions are executed on in the same "UI" thread and do not need
// additional synchronization.
func (p *TermstatusProgressUI) StartPhase(progress func() string, status func() []string, percent func() (int64, int64), summary func(time.Duration)) {
	p.updates <- func() {
		p.diplaySummary() // display summary of the prior phase if any
		p.phase = progressPhase{
			stopwatch: startStopwatch(),
			progress:  progress,
			status:    status,
			percent:   percent,
			summary:   summary,
		}
		p.displayProgress(true) // display initial progress
	}
}

// FinishPhase stops periodic progress report for the current phase and prints phase execution summary.
func (p *TermstatusProgressUI) FinishPhase() {
	p.updates <- func() {
		p.diplaySummary() // display summary of the prior phase if any
		p.phase = progressPhase{}
		p.displayProgress(false)
	}
}

// Finish stops periodic progress report for the current phase, prints phase execution summary,
// then stops this TermstatusProgressUI
func (p *TermstatusProgressUI) Finish() {
	close(p.updates)

	<-p.running // let the worker finish what it's doing
}

// diplaySummary of the current phase, clears progress/status as necessary
func (p *TermstatusProgressUI) diplaySummary() {
	p.term.SetStatus([]string{})
	if p.phase.summary != nil {
		p.phase.summary(p.phase.stopwatch.elapsed())
	}
}

// decorateProgress message with running time, completion % and ETA, if available
func (p *TermstatusProgressUI) decorateProgress() string {
	line := fmt.Sprintf("[%s] %s", FormatDuration(p.phase.stopwatch.elapsed()), p.phase.progress())
	if p.phase.percent != nil {
		current, total := p.phase.percent()
		line = line + fmt.Sprintf(" %s ETA %s", FormatPercent(uint64(current), uint64(total)), FormatDuration(eta(p.phase.stopwatch, current, total)))
	}
	return line
}

// displayInteructiveProgress reports operation phase progress on a terminal supported by termstatus
func (p *TermstatusProgressUI) displayInteructiveProgress() {
	// TODO asci-art progress bar, if completion percent is available
	lines := []string{p.decorateProgress()}
	if p.phase.status != nil {
		lines = append(lines, p.phase.status()...)
	}
	p.term.SetStatus(lines)
}

// displayProgress prints transient progress and status information about current phase of a long running command
func (p *TermstatusProgressUI) displayProgress(first bool) {
	if p.phase.progress == nil {
		return
	}

	if p.term.CanDisplayStatus() {
		p.displayInteructiveProgress()
	} else {
		// dumb terminal print progress message only, no status lines
		p.V("%s", p.decorateProgress())
	}
}

// stopwatch knows how to tell elapsed time since it was started
type stopwatch time.Time

// startStopwatch returns running stopwatch instances
func startStopwatch() stopwatch {
	return stopwatch(time.Now())
}

// elapsed returns time duration since the stopwatch was started
func (s stopwatch) elapsed() time.Duration {
	return time.Since(time.Time(s))
}

// FormatBytes formats provided number in best matching binary units (B/KiB/MiB/etc)
func FormatBytes(c int64) string {
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

// FormatDuration renders duration as user-friendly HH:mm:ss string
func FormatDuration(d time.Duration) string {
	sec := uint64(d / time.Second)
	return FormatSeconds(sec)
}

func eta(sw stopwatch, current int64, total int64) time.Duration {
	if current >= total {
		return etaDONE
	}

	elapsed := sw.elapsed()

	if elapsed <= 0 || current <= 0 {
		return etaNA
	}

	// can't calculate in nanoseconds because int64 can overflow
	// will calculate in float64 seconds, then convert back to nanoseconds
	etaSec := elapsed.Seconds() * (float64(total) - float64(current)) / float64(current)
	eta := time.Duration(etaSec * float64(time.Second.Nanoseconds()))

	// fmt.Printf("elapsed=%d current=%d target=%d etaSec=%f eta=%d\n", elapsed, current, target, etaSec, eta)

	return eta
}

// FormatETA ETA string
func formatETA(eta time.Duration) string {
	switch {
	case eta == etaDONE:
		return "DONE"
	case eta == etaNA:
		return "N/A"
	default:
		return FormatDuration(eta)
	}
}
