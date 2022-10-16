package backup

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/restic/restic/internal/archiver"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/signals"
)

type ProgressPrinter interface {
	Update(total, processed Counter, errors uint, currentFiles map[string]struct{}, start time.Time, secs uint64)
	Error(item string, err error) error
	ScannerError(item string, err error) error
	CompleteItem(messageType string, item string, previous, current *restic.Node, s archiver.ItemStats, d time.Duration)
	ReportTotal(item string, start time.Time, s archiver.ScanStats)
	Finish(snapshotID restic.ID, start time.Time, summary *Summary, dryRun bool)
	Reset()

	// ui.StdioWrapper
	Stdout() io.WriteCloser
	Stderr() io.WriteCloser

	P(msg string, args ...interface{})
	V(msg string, args ...interface{})
}

type Counter struct {
	Files, Dirs, Bytes uint64
}

type fileWorkerMessage struct {
	filename string
	done     bool
}

type Summary struct {
	sync.Mutex
	Files, Dirs struct {
		New       uint
		Changed   uint
		Unchanged uint
	}
	ProcessedBytes uint64
	archiver.ItemStats
}

// Progress reports progress for the `backup` command.
type Progress struct {
	interval time.Duration
	start    time.Time

	totalCh     chan Counter
	processedCh chan Counter
	errCh       chan struct{}
	workerCh    chan fileWorkerMessage
	closed      chan struct{}

	summary Summary
	printer ProgressPrinter
}

func NewProgress(printer ProgressPrinter, interval time.Duration) *Progress {
	return &Progress{
		interval: interval,
		start:    time.Now(),

		// use buffered channels for the information used to update the status
		// the shutdown of the `Run()` method is somewhat racy, but won't affect
		// the final backup statistics
		totalCh:     make(chan Counter, 100),
		processedCh: make(chan Counter, 100),
		errCh:       make(chan struct{}),
		workerCh:    make(chan fileWorkerMessage, 100),
		closed:      make(chan struct{}),

		printer: printer,
	}
}

// Run regularly updates the status lines. It should be called in a separate
// goroutine.
func (p *Progress) Run(ctx context.Context) {
	var (
		lastUpdate       time.Time
		total, processed Counter
		errors           uint
		started          bool
		currentFiles     = make(map[string]struct{})
		secondsRemaining uint64
	)

	t := time.NewTicker(time.Second)
	signalsCh := signals.GetProgressChannel()
	defer t.Stop()
	defer close(p.closed)
	// Reset status when finished
	defer p.printer.Reset()

	for {
		forceUpdate := false
		select {
		case <-ctx.Done():
			return
		case t, ok := <-p.totalCh:
			if ok {
				total = t
				started = true
			} else {
				// scan has finished
				p.totalCh = nil
			}
		case s := <-p.processedCh:
			processed.Files += s.Files
			processed.Dirs += s.Dirs
			processed.Bytes += s.Bytes
			started = true
		case <-p.errCh:
			errors++
			started = true
		case m := <-p.workerCh:
			if m.done {
				delete(currentFiles, m.filename)
			} else {
				currentFiles[m.filename] = struct{}{}
			}
		case <-t.C:
			if !started {
				continue
			}

			if p.totalCh == nil {
				secs := float64(time.Since(p.start) / time.Second)
				todo := float64(total.Bytes - processed.Bytes)
				secondsRemaining = uint64(secs / float64(processed.Bytes) * todo)
			}
		case <-signalsCh:
			forceUpdate = true
		}

		// limit update frequency
		if !forceUpdate && (p.interval == 0 || time.Since(lastUpdate) < p.interval) {
			continue
		}
		lastUpdate = time.Now()

		p.printer.Update(total, processed, errors, currentFiles, p.start, secondsRemaining)
	}
}

// Error is the error callback function for the archiver, it prints the error and returns nil.
func (p *Progress) Error(item string, err error) error {
	cbErr := p.printer.Error(item, err)

	select {
	case p.errCh <- struct{}{}:
	case <-p.closed:
	}
	return cbErr
}

// StartFile is called when a file is being processed by a worker.
func (p *Progress) StartFile(filename string) {
	select {
	case p.workerCh <- fileWorkerMessage{filename: filename}:
	case <-p.closed:
	}
}

// CompleteBlob is called for all saved blobs for files.
func (p *Progress) CompleteBlob(bytes uint64) {
	select {
	case p.processedCh <- Counter{Bytes: bytes}:
	case <-p.closed:
	}
}

// CompleteItem is the status callback function for the archiver when a
// file/dir has been saved successfully.
func (p *Progress) CompleteItem(item string, previous, current *restic.Node, s archiver.ItemStats, d time.Duration) {
	p.summary.Lock()
	p.summary.ItemStats.Add(s)

	// for the last item "/", current is nil
	if current != nil {
		p.summary.ProcessedBytes += current.Size
	}

	p.summary.Unlock()

	if current == nil {
		// error occurred, tell the status display to remove the line
		select {
		case p.workerCh <- fileWorkerMessage{filename: item, done: true}:
		case <-p.closed:
		}
		return
	}

	switch current.Type {
	case "dir":
		select {
		case p.processedCh <- Counter{Dirs: 1}:
		case <-p.closed:
		}

		if previous == nil {
			p.printer.CompleteItem("dir new", item, previous, current, s, d)
			p.summary.Lock()
			p.summary.Dirs.New++
			p.summary.Unlock()
			return
		}

		if previous.Equals(*current) {
			p.printer.CompleteItem("dir unchanged", item, previous, current, s, d)
			p.summary.Lock()
			p.summary.Dirs.Unchanged++
			p.summary.Unlock()
		} else {
			p.printer.CompleteItem("dir modified", item, previous, current, s, d)
			p.summary.Lock()
			p.summary.Dirs.Changed++
			p.summary.Unlock()
		}

	case "file":
		select {
		case p.processedCh <- Counter{Files: 1}:
		case <-p.closed:
		}
		select {
		case p.workerCh <- fileWorkerMessage{filename: item, done: true}:
		case <-p.closed:
		}

		if previous == nil {
			p.printer.CompleteItem("file new", item, previous, current, s, d)
			p.summary.Lock()
			p.summary.Files.New++
			p.summary.Unlock()
			return
		}

		if previous.Equals(*current) {
			p.printer.CompleteItem("file unchanged", item, previous, current, s, d)
			p.summary.Lock()
			p.summary.Files.Unchanged++
			p.summary.Unlock()
		} else {
			p.printer.CompleteItem("file modified", item, previous, current, s, d)
			p.summary.Lock()
			p.summary.Files.Changed++
			p.summary.Unlock()
		}
	}
}

// ReportTotal sets the total stats up to now
func (p *Progress) ReportTotal(item string, s archiver.ScanStats) {
	select {
	case p.totalCh <- Counter{Files: uint64(s.Files), Dirs: uint64(s.Dirs), Bytes: s.Bytes}:
	case <-p.closed:
	}

	if item == "" {
		p.printer.ReportTotal(item, p.start, s)
		close(p.totalCh)
		return
	}
}

// Finish prints the finishing messages.
func (p *Progress) Finish(snapshotID restic.ID, dryrun bool) {
	// wait for the status update goroutine to shut down
	<-p.closed
	p.printer.Finish(snapshotID, p.start, &p.summary, dryrun)
}
