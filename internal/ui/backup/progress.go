package backup

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/restic/restic/internal/archiver"
	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/progress"
)

// A ProgressPrinter can print various progress messages.
// It must be safe to call its methods from concurrent goroutines.
type ProgressPrinter interface {
	Update(total, processed Counter, errors uint32, currentFiles map[string]struct{}, start time.Time, secs uint64)
	Error(item string, err error) error
	ScannerError(item string, err error) error
	CompleteItem(messageType string, item string, s *archiver.ItemStats, d time.Duration)
	ReportTotal(start time.Time, s archiver.ScanStats)
	Finish(snapshotID restic.ID, summary *archiver.Summary, dryRun bool)
	Reset()

	progress.Printer
}

type Counter struct {
	Files, Dirs, Bytes atomic.Uint64
}

// Progress reports progress for the `backup` command.
type Progress struct {
	progress.Updater
	mu sync.Mutex

	start     time.Time
	estimator rateEstimator

	scanStarted, scanFinished atomic.Bool

	currentFiles     map[string]struct{}
	processed, total Counter
	errors           atomic.Uint32

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
			if !p.scanStarted.Load() {
				return
			}

			var secondsRemaining uint64
			if p.scanFinished.Load() {
				rate := p.estimator.rate(time.Now())
				tooSlowCutoff := 1024.
				if rate <= tooSlowCutoff {
					secondsRemaining = 0
				} else {
					todo := float64(p.total.Bytes.Load() - p.processed.Bytes.Load())
					secondsRemaining = uint64(todo / rate)
				}
			}

			p.mu.Lock()
			defer p.mu.Unlock()
			p.printer.Update(p.total, p.processed, p.errors.Load(), p.currentFiles, p.start, secondsRemaining)
		}
	})
	return p
}

// Error is the error callback function for the archiver, it prints the error and returns nil.
func (p *Progress) Error(item string, err error) error {
	p.errors.Add(1)
	p.scanStarted.Store(true)

	return p.printer.Error(item, err)
}

// StartFile is called when a file is being processed by a worker.
func (p *Progress) StartFile(filename string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.currentFiles[filename] = struct{}{}
}

// CompleteBlob is called for all saved blobs for files.
func (p *Progress) CompleteBlob(bytes uint64) {
	p.processed.Bytes.Add(bytes)
	p.estimator.recordBytes(time.Now(), bytes)
	p.scanStarted.Store(true)
}

// CompleteItem is the status callback function for the archiver when a
// file/dir has been saved successfully.
func (p *Progress) CompleteItem(item string, previous, current *data.Node, s *archiver.ItemStats, d time.Duration) {
	if current == nil {
		// error occurred, tell the status display to remove the line
		p.mu.Lock()
		delete(p.currentFiles, item)
		p.mu.Unlock()
		return
	}

	switch current.Type {
	case data.NodeTypeDir:
		p.processed.Dirs.Add(1)
		p.scanStarted.Store(true)

		switch {
		case previous == nil:
			p.printer.CompleteItem("dir new", item, s, d)
		case previous.Equals(*current):
			p.printer.CompleteItem("dir unchanged", item, s, d)
		default:
			p.printer.CompleteItem("dir modified", item, s, d)
		}

	case data.NodeTypeFile:
		p.processed.Files.Add(1)
		p.scanStarted.Store(true)

		p.mu.Lock()
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
	p.total.Files.Store(uint64(s.Files))
	p.total.Dirs.Store(uint64(s.Dirs))
	p.total.Bytes.Store(uint64(s.Bytes))
	p.scanStarted.Store(true)

	if item == "" {
		p.scanFinished.Store(true)
		p.printer.ReportTotal(p.start, s)
	}
}

// Finish prints the finishing messages.
func (p *Progress) Finish(snapshotID restic.ID, summary *archiver.Summary, dryrun bool) {
	// wait for the status update goroutine to shut down
	p.Updater.Done()
	p.printer.Finish(snapshotID, summary, dryrun)
}
