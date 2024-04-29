package backup

import (
	"sync"
	"time"

	"github.com/restic/restic/internal/archiver"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/progress"
)

// A ProgressPrinter can print various progress messages.
// It must be safe to call its methods from concurrent goroutines.
type ProgressPrinter interface {
	Update(total, processed Counter, errors uint, currentFiles map[string]struct{}, start time.Time, secs uint64)
	Error(item string, err error) error
	ScannerError(item string, err error) error
	CompleteItem(messageType string, item string, s archiver.ItemStats, d time.Duration)
	ReportTotal(start time.Time, s archiver.ScanStats)
	Finish(snapshotID restic.ID, start time.Time, summary *archiver.Summary, dryRun bool)
	Reset()

	P(msg string, args ...interface{})
	V(msg string, args ...interface{})
}

type Counter struct {
	Files, Dirs, Bytes uint64
}

// Progress reports progress for the `backup` command.
type Progress struct {
	progress.Updater
	mu sync.Mutex

	start     time.Time
	estimator rateEstimator

	scanStarted, scanFinished bool

	currentFiles     map[string]struct{}
	processed, total Counter
	errors           uint

	printer ProgressPrinter
}

func NewProgress(printer ProgressPrinter, interval time.Duration) *Progress {
	p := &Progress{
		start:        time.Now(),
		currentFiles: make(map[string]struct{}),
		printer:      printer,
		estimator:    *newRateEstimator(time.Now()),
	}
	p.Updater = *progress.NewUpdater(interval, func(_ time.Duration, final bool) {
		if final {
			p.printer.Reset()
		} else {
			p.mu.Lock()
			defer p.mu.Unlock()
			if !p.scanStarted {
				return
			}

			var secondsRemaining uint64
			if p.scanFinished {
				rate := p.estimator.rate(time.Now())
				tooSlowCutoff := 1024.
				if rate <= tooSlowCutoff {
					secondsRemaining = 0
				} else {
					todo := float64(p.total.Bytes - p.processed.Bytes)
					secondsRemaining = uint64(todo / rate)
				}
			}

			p.printer.Update(p.total, p.processed, p.errors, p.currentFiles, p.start, secondsRemaining)
		}
	})
	return p
}

// Error is the error callback function for the archiver, it prints the error and returns nil.
func (p *Progress) Error(item string, err error) error {
	p.mu.Lock()
	p.errors++
	p.scanStarted = true
	p.mu.Unlock()

	return p.printer.Error(item, err)
}

// StartFile is called when a file is being processed by a worker.
func (p *Progress) StartFile(filename string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.currentFiles[filename] = struct{}{}
}

func (p *Progress) addProcessed(c Counter) {
	p.processed.Files += c.Files
	p.processed.Dirs += c.Dirs
	p.processed.Bytes += c.Bytes
	p.estimator.recordBytes(time.Now(), c.Bytes)
	p.scanStarted = true
}

// CompleteBlob is called for all saved blobs for files.
func (p *Progress) CompleteBlob(bytes uint64) {
	p.mu.Lock()
	p.addProcessed(Counter{Bytes: bytes})
	p.mu.Unlock()
}

// CompleteItem is the status callback function for the archiver when a
// file/dir has been saved successfully.
func (p *Progress) CompleteItem(item string, previous, current *restic.Node, s archiver.ItemStats, d time.Duration) {
	if current == nil {
		// error occurred, tell the status display to remove the line
		p.mu.Lock()
		delete(p.currentFiles, item)
		p.mu.Unlock()
		return
	}

	switch current.Type {
	case "dir":
		p.mu.Lock()
		p.addProcessed(Counter{Dirs: 1})
		p.mu.Unlock()

		switch {
		case previous == nil:
			p.printer.CompleteItem("dir new", item, s, d)
		case previous.Equals(*current):
			p.printer.CompleteItem("dir unchanged", item, s, d)
		default:
			p.printer.CompleteItem("dir modified", item, s, d)
		}

	case "file":
		p.mu.Lock()
		p.addProcessed(Counter{Files: 1})
		delete(p.currentFiles, item)
		p.mu.Unlock()

		switch {
		case previous == nil:
			p.printer.CompleteItem("file new", item, s, d)
		case previous.Equals(*current):
			p.printer.CompleteItem("file unchanged", item, s, d)
		default:
			p.printer.CompleteItem("file modified", item, s, d)
		}
	}
}

// ReportTotal sets the total stats up to now
func (p *Progress) ReportTotal(item string, s archiver.ScanStats) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.total = Counter{Files: uint64(s.Files), Dirs: uint64(s.Dirs), Bytes: s.Bytes}
	p.scanStarted = true

	if item == "" {
		p.scanFinished = true
		p.printer.ReportTotal(p.start, s)
	}
}

// Finish prints the finishing messages.
func (p *Progress) Finish(snapshotID restic.ID, summary *archiver.Summary, dryrun bool) {
	// wait for the status update goroutine to shut down
	p.Updater.Done()
	p.printer.Finish(snapshotID, p.start, summary, dryrun)
}
