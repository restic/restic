package ui

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/restic/restic/internal/archiver"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/termstatus"
	"sync"
)

// Restore reports progress for the `restore` command.
type Restore struct {
	*Message
	*StdioWrapper

	MinUpdatePause time.Duration

	term  *termstatus.Terminal
	v     uint
	start time.Time

	totalBytes uint64

	totalCh     chan counter
	processedCh chan counter
	errCh       chan struct{}
	workerCh    chan fileWorkerMessage
	finished    chan struct{}

	total, processed counter
	errors           uint
	eta              uint64

	summary struct {
		sync.Mutex
		Files, Dirs struct {
			New       uint
			Changed   uint
			Unchanged uint
		}
		archiver.ItemStats
	}
}

// NewRestore returns a new restoration progress reporter.
func NewRestore(term *termstatus.Terminal, verbosity uint) *Restore {
	ret := &Restore{
		Message:      NewMessage(term, verbosity),
		StdioWrapper: NewStdioWrapper(term),
		term:         term,
		v:            verbosity,
		start:        time.Now(),

		// limit to 60fps by default
		MinUpdatePause: time.Second / 60,

		totalCh:     make(chan counter),
		processedCh: make(chan counter),
		errCh:       make(chan struct{}),
		workerCh:    make(chan fileWorkerMessage),
		finished:    make(chan struct{}),
	}
	return ret
}

// Run regularly updates the status lines. It should be called in a separate
// goroutine.
func (b *Restore) Run(ctx context.Context) error {
	var (
		lastUpdate   time.Time
		started      bool
		currentFiles = make(map[string]struct{})
	)

	t := time.NewTicker(time.Second)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-b.finished:
			started = false
			b.term.SetStatus([]string{""})
		case t, ok := <-b.totalCh:
			if ok {
				b.total = t
				started = true
			} else {
				// scan has finished
				b.totalCh = nil
				b.totalBytes = b.total.Bytes
			}
		case s := <-b.processedCh:
			b.processed.Files += s.Files
			b.processed.Dirs += s.Dirs
			b.processed.Bytes += s.Bytes
			started = true
		case <-b.errCh:
			b.errors++
			started = true
		case m := <-b.workerCh:
			if m.done {
				delete(currentFiles, m.filename)
			} else {
				currentFiles[m.filename] = struct{}{}
			}
		case <-t.C:
			if !started {
				continue
			}

			if b.totalCh == nil {
				secs := float64(time.Since(b.start) / time.Second)
				todo := float64(b.total.Bytes - b.processed.Bytes)
				b.eta = uint64(secs / float64(b.processed.Bytes) * todo)
			}
		}

		// limit update frequency
		if time.Since(lastUpdate) < b.MinUpdatePause {
			continue
		}
		lastUpdate = time.Now()

		b.update(currentFiles)
	}
}

// update updates the status lines.
func (b *Restore) update(currentFiles map[string]struct{}) {
	var status string
	if b.total.Files == 0 && b.total.Dirs == 0 {
		// no total count available yet
		status = fmt.Sprintf("[%s] %v files, %s, %d errors",
			formatDuration(time.Since(b.start)),
			b.processed.Files, formatBytes(b.processed.Bytes), b.errors,
		)
	} else {
		var eta, percent string

		if b.eta > 0 {
			eta = fmt.Sprintf(" ETA %s", formatSeconds(b.eta))
		}
		if b.processed.Bytes < b.total.Bytes {
			percent = formatPercent(b.processed.Bytes, b.total.Bytes)
			percent += "  "
		}

		// include totals
		status = fmt.Sprintf("[%s] %s%v files %s, total %v files %v, %d errors%s",
			formatDuration(time.Since(b.start)),
			percent,
			b.processed.Files,
			formatBytes(b.processed.Bytes),
			b.total.Files,
			formatBytes(b.total.Bytes),
			b.errors,
			eta,
		)
	}

	lines := make([]string, 0, len(currentFiles)+1)
	for filename := range currentFiles {
		lines = append(lines, filename)
	}
	sort.Sort(sort.StringSlice(lines))
	lines = append([]string{status}, lines...)

	b.term.SetStatus(lines)
}

// ScannerError is the error callback function for the scanner, it prints the
// error in verbose mode and returns nil.
func (b *Restore) ScannerError(item string, fi os.FileInfo, err error) error {
	b.V("scan: %v\n", err)
	return nil
}

// Error is the error callback function for the archiver, it prints the error and returns nil.
func (b *Restore) Error(msg string, args ...interface{}) error {
	b.E(msg, args...)
	b.errCh <- struct{}{}
	return nil
}

// StartFile is called when a file is being processed by a worker.
func (b *Restore) StartFile(filename string) {
	b.workerCh <- fileWorkerMessage{
		filename: filename,
	}
}

// CompleteBlob is called for all saved blobs for files.
func (b *Restore) CompleteBlob(filename string, bytes uint64) {
	b.processedCh <- counter{Bytes: bytes}
}

// CompleteItemFn is the status callback function for the archiver when a
// file/dir has been saved successfully.
func (b *Restore) CompleteItemFn(item string, previous, current *restic.Node, s archiver.ItemStats, d time.Duration) {
	b.summary.Lock()
	b.summary.ItemStats.Add(s)
	b.summary.Unlock()

	if current == nil {
		// error occurred, tell the status display to remove the line
		b.workerCh <- fileWorkerMessage{
			filename: item,
			done:     true,
		}
		return
	}

	switch current.Type {
	case "file":
		b.processedCh <- counter{Files: 1}
		b.workerCh <- fileWorkerMessage{
			filename: item,
			done:     true,
		}
	case "dir":
		b.processedCh <- counter{Dirs: 1}
	}

	if current.Type == "dir" {
		if previous == nil {
			b.VV("new       %v, saved in %.3fs (%v added, %v metadata)", item, d.Seconds(), formatBytes(s.DataSize), formatBytes(s.TreeSize))
			b.summary.Lock()
			b.summary.Dirs.New++
			b.summary.Unlock()
			return
		}

		if previous.Equals(*current) {
			b.VV("unchanged %v", item)
			b.summary.Lock()
			b.summary.Dirs.Unchanged++
			b.summary.Unlock()
		} else {
			b.VV("modified  %v, saved in %.3fs (%v added, %v metadata)", item, d.Seconds(), formatBytes(s.DataSize), formatBytes(s.TreeSize))
			b.summary.Lock()
			b.summary.Dirs.Changed++
			b.summary.Unlock()
		}

	} else if current.Type == "file" {

		b.workerCh <- fileWorkerMessage{
			done:     true,
			filename: item,
		}

		if previous == nil {
			b.VV("new       %v, saved in %.3fs (%v added)", item, d.Seconds(), formatBytes(s.DataSize))
			b.summary.Lock()
			b.summary.Files.New++
			b.summary.Unlock()
			return
		}

		if previous.Equals(*current) {
			b.VV("unchanged %v", item)
			b.summary.Lock()
			b.summary.Files.Unchanged++
			b.summary.Unlock()
		} else {
			b.VV("modified  %v, saved in %.3fs (%v added)", item, d.Seconds(), formatBytes(s.DataSize))
			b.summary.Lock()
			b.summary.Files.Changed++
			b.summary.Unlock()
		}
	}
}

// ReportTotal sets the total stats up to now
func (b *Restore) ReportTotal(item string, s archiver.ScanStats) {
	b.totalCh <- counter{Files: s.Files, Dirs: s.Dirs, Bytes: s.Bytes}

	if item == "" {
		b.V("scan finished in %.3fs: %v files, %s",
			time.Since(b.start).Seconds(),
			s.Files, formatBytes(s.Bytes),
		)
		close(b.totalCh)
		return
	}
}

// Finish prints the finishing messages.
func (b *Restore) Finish() {
	close(b.finished)

	b.P("\n")
	b.P("Files:       %5d new, %5d changed, %5d unmodified\n", b.summary.Files.New, b.summary.Files.Changed, b.summary.Files.Unchanged)
	// r.P("Dirs:        %5d new, %5d changed, %5d unmodified\n", r.summary.Dirs.New, r.summary.Dirs.Changed, r.summary.Dirs.Unchanged)
	b.V("Data Blobs:  %5d new\n", b.summary.ItemStats.DataBlobs)
	b.V("Tree Blobs:  %5d new\n", b.summary.ItemStats.TreeBlobs)
	b.P("Added:      %-5s\n", formatBytes(b.summary.ItemStats.DataSize+b.summary.ItemStats.TreeSize))
	b.P("Errors:      %5d\n", b.errors)
	b.P("\n")
	b.P("processed %v files, %v in %s",
		b.summary.Files.New+b.summary.Files.Changed+b.summary.Files.Unchanged,
		formatBytes(b.totalBytes),
		formatDuration(time.Since(b.start)),
	)
}
