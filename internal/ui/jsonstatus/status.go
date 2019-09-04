package jsonstatus

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/restic/restic/internal/archiver"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/termstatus"
)

type counter struct {
	Files, Dirs, Bytes uint64
}

type fileWorkerMessage struct {
	filename string
	done     bool
}

// Backup reports progress for the `backup` command in JSON.
type Backup struct {
	*ui.Message
	*ui.StdioWrapper

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

// NewBackup returns a new backup progress reporter.
func NewBackup(term *termstatus.Terminal, verbosity uint) *Backup {
	return &Backup{
		Message:      ui.NewMessage(term, verbosity),
		StdioWrapper: ui.NewStdioWrapper(term),
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
}

// Run regularly updates the status lines. It should be called in a separate
// goroutine.
func (b *Backup) Run(ctx context.Context) error {
	var (
		lastUpdate       time.Time
		total, processed counter
		errors           uint
		started          bool
		currentFiles     = make(map[string]struct{})
		secondsRemaining uint64
	)

	t := time.NewTicker(time.Second)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-b.finished:
			started = false
		case t, ok := <-b.totalCh:
			if ok {
				total = t
				started = true
			} else {
				// scan has finished
				b.totalCh = nil
				b.totalBytes = total.Bytes
			}
		case s := <-b.processedCh:
			processed.Files += s.Files
			processed.Dirs += s.Dirs
			processed.Bytes += s.Bytes
			started = true
		case <-b.errCh:
			errors++
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
				todo := float64(total.Bytes - processed.Bytes)
				secondsRemaining = uint64(secs / float64(processed.Bytes) * todo)
			}
		}

		// limit update frequency
		if time.Since(lastUpdate) < b.MinUpdatePause {
			continue
		}
		lastUpdate = time.Now()

		b.update(total, processed, errors, currentFiles, secondsRemaining)
	}
}

// update updates the status lines.
func (b *Backup) update(total, processed counter, errors uint, currentFiles map[string]struct{}, secs uint64) {
	status := statusUpdate{
		MessageType:      "status",
		SecondsElapsed:   uint64(time.Since(b.start) / time.Second),
		SecondsRemaining: secs,
		TotalFiles:       total.Files,
		FilesDone:        processed.Files,
		TotalBytes:       total.Bytes,
		BytesDone:        processed.Bytes,
		ErrorCount:       errors,
	}

	if total.Bytes > 0 {
		status.PercentDone = float64(processed.Bytes) / float64(total.Bytes)
	}

	for filename := range currentFiles {
		status.CurrentFiles = append(status.CurrentFiles, filename)
	}
	sort.Strings(status.CurrentFiles)

	json.NewEncoder(b.StdioWrapper.Stdout()).Encode(status)
}

// ScannerError is the error callback function for the scanner, it prints the
// error in verbose mode and returns nil.
func (b *Backup) ScannerError(item string, fi os.FileInfo, err error) error {
	json.NewEncoder(b.StdioWrapper.Stderr()).Encode(errorUpdate{
		MessageType: "error",
		Error:       err,
		During:      "scan",
		Item:        item,
	})
	return nil
}

// Error is the error callback function for the archiver, it prints the error and returns nil.
func (b *Backup) Error(item string, fi os.FileInfo, err error) error {
	json.NewEncoder(b.StdioWrapper.Stderr()).Encode(errorUpdate{
		MessageType: "error",
		Error:       err,
		During:      "archival",
		Item:        item,
	})
	b.errCh <- struct{}{}
	return nil
}

// StartFile is called when a file is being processed by a worker.
func (b *Backup) StartFile(filename string) {
	b.workerCh <- fileWorkerMessage{
		filename: filename,
	}
}

// CompleteBlob is called for all saved blobs for files.
func (b *Backup) CompleteBlob(filename string, bytes uint64) {
	b.processedCh <- counter{Bytes: bytes}
}

// CompleteItem is the status callback function for the archiver when a
// file/dir has been saved successfully.
func (b *Backup) CompleteItem(item string, previous, current *restic.Node, s archiver.ItemStats, d time.Duration) {
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
			if b.v >= 3 {
				json.NewEncoder(b.StdioWrapper.Stdout()).Encode(verboseUpdate{
					MessageType:  "verbose_status",
					Action:       "new",
					Item:         item,
					Duration:     d.Seconds(),
					DataSize:     s.DataSize,
					MetadataSize: s.TreeSize,
				})
			}
			b.summary.Lock()
			b.summary.Dirs.New++
			b.summary.Unlock()
			return
		}

		if previous.Equals(*current) {
			if b.v >= 3 {
				json.NewEncoder(b.StdioWrapper.Stdout()).Encode(verboseUpdate{
					MessageType: "verbose_status",
					Action:      "unchanged",
					Item:        item,
				})
			}
			b.summary.Lock()
			b.summary.Dirs.Unchanged++
			b.summary.Unlock()
		} else {
			if b.v >= 3 {
				json.NewEncoder(b.StdioWrapper.Stdout()).Encode(verboseUpdate{
					MessageType:  "verbose_status",
					Action:       "modified",
					Item:         item,
					Duration:     d.Seconds(),
					DataSize:     s.DataSize,
					MetadataSize: s.TreeSize,
				})
			}
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
			if b.v >= 3 {
				json.NewEncoder(b.StdioWrapper.Stdout()).Encode(verboseUpdate{
					MessageType: "verbose_status",
					Action:      "new",
					Item:        item,
					Duration:    d.Seconds(),
					DataSize:    s.DataSize,
				})
			}
			b.summary.Lock()
			b.summary.Files.New++
			b.summary.Unlock()
			return
		}

		if previous.Equals(*current) {
			if b.v >= 3 {
				json.NewEncoder(b.StdioWrapper.Stdout()).Encode(verboseUpdate{
					MessageType: "verbose_status",
					Action:      "unchanged",
					Item:        item,
				})
			}
			b.summary.Lock()
			b.summary.Files.Unchanged++
			b.summary.Unlock()
		} else {
			if b.v >= 3 {
				json.NewEncoder(b.StdioWrapper.Stdout()).Encode(verboseUpdate{
					MessageType: "verbose_status",
					Action:      "modified",
					Item:        item,
					Duration:    d.Seconds(),
					DataSize:    s.DataSize,
				})
			}
			b.summary.Lock()
			b.summary.Files.Changed++
			b.summary.Unlock()
		}
	}
}

// ReportTotal sets the total stats up to now
func (b *Backup) ReportTotal(item string, s archiver.ScanStats) {
	select {
	case b.totalCh <- counter{Files: uint64(s.Files), Dirs: uint64(s.Dirs), Bytes: s.Bytes}:
	case <-b.finished:
	}

	if item == "" {
		if b.v >= 2 {
			json.NewEncoder(b.StdioWrapper.Stdout()).Encode(verboseUpdate{
				MessageType: "status",
				Action:      "scan_finished",
				Duration:    time.Since(b.start).Seconds(),
				DataSize:    s.Bytes,
				TotalFiles:  s.Files,
			})
		}
		close(b.totalCh)
		return
	}
}

// Finish prints the finishing messages.
func (b *Backup) Finish(snapshotID restic.ID) {
	close(b.finished)
	json.NewEncoder(b.StdioWrapper.Stdout()).Encode(summaryOutput{
		MessageType:         "summary",
		FilesNew:            b.summary.Files.New,
		FilesChanged:        b.summary.Files.Changed,
		FilesUnmodified:     b.summary.Files.Unchanged,
		DirsNew:             b.summary.Dirs.New,
		DirsChanged:         b.summary.Dirs.Changed,
		DirsUnmodified:      b.summary.Dirs.Unchanged,
		DataBlobs:           b.summary.ItemStats.DataBlobs,
		TreeBlobs:           b.summary.ItemStats.TreeBlobs,
		DataAdded:           b.summary.ItemStats.DataSize + b.summary.ItemStats.TreeSize,
		TotalFilesProcessed: b.summary.Files.New + b.summary.Files.Changed + b.summary.Files.Unchanged,
		TotalBytesProcessed: b.totalBytes,
		TotalDuration:       time.Since(b.start).Seconds(),
		SnapshotID:          snapshotID.Str(),
	})
}

// SetMinUpdatePause sets b.MinUpdatePause. It satisfies the
// ArchiveProgressReporter interface.
func (b *Backup) SetMinUpdatePause(d time.Duration) {
	b.MinUpdatePause = d
}

type statusUpdate struct {
	MessageType      string   `json:"message_type"` // "status"
	SecondsElapsed   uint64   `json:"seconds_elapsed,omitempty"`
	SecondsRemaining uint64   `json:"seconds_remaining,omitempty"`
	PercentDone      float64  `json:"percent_done"`
	TotalFiles       uint64   `json:"total_files,omitempty"`
	FilesDone        uint64   `json:"files_done,omitempty"`
	TotalBytes       uint64   `json:"total_bytes,omitempty"`
	BytesDone        uint64   `json:"bytes_done,omitempty"`
	ErrorCount       uint     `json:"error_count,omitempty"`
	CurrentFiles     []string `json:"current_files,omitempty"`
}

type errorUpdate struct {
	MessageType string `json:"message_type"` // "error"
	Error       error  `json:"error"`
	During      string `json:"during"`
	Item        string `json:"item"`
}

type verboseUpdate struct {
	MessageType  string  `json:"message_type"` // "verbose_status"
	Action       string  `json:"action"`
	Item         string  `json:"item"`
	Duration     float64 `json:"duration"` // in seconds
	DataSize     uint64  `json:"data_size"`
	MetadataSize uint64  `json:"metadata_size"`
	TotalFiles   uint    `json:"total_files"`
}

type summaryOutput struct {
	MessageType         string  `json:"message_type"` // "summary"
	FilesNew            uint    `json:"files_new"`
	FilesChanged        uint    `json:"files_changed"`
	FilesUnmodified     uint    `json:"files_unmodified"`
	DirsNew             uint    `json:"dirs_new"`
	DirsChanged         uint    `json:"dirs_changed"`
	DirsUnmodified      uint    `json:"dirs_unmodified"`
	DataBlobs           int     `json:"data_blobs"`
	TreeBlobs           int     `json:"tree_blobs"`
	DataAdded           uint64  `json:"data_added"`
	TotalFilesProcessed uint    `json:"total_files_processed"`
	TotalBytesProcessed uint64  `json:"total_bytes_processed"`
	TotalDuration       float64 `json:"total_duration"` // in seconds
	SnapshotID          string  `json:"snapshot_id"`
}
