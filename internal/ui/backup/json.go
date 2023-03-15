package backup

import (
	"bytes"
	"encoding/json"
	"sort"
	"time"

	"github.com/restic/restic/internal/archiver"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/termstatus"
)

// JSONProgress reports progress for the `backup` command in JSON.
type JSONProgress struct {
	*ui.Message

	term *termstatus.Terminal
	v    uint
}

// assert that Backup implements the ProgressPrinter interface
var _ ProgressPrinter = &JSONProgress{}

// NewJSONProgress returns a new backup progress reporter.
func NewJSONProgress(term *termstatus.Terminal, verbosity uint) *JSONProgress {
	return &JSONProgress{
		Message: ui.NewMessage(term, verbosity),
		term:    term,
		v:       verbosity,
	}
}

func toJSONString(status interface{}) string {
	buf := new(bytes.Buffer)
	err := json.NewEncoder(buf).Encode(status)
	if err != nil {
		panic(err)
	}
	return buf.String()
}

func (b *JSONProgress) print(status interface{}) {
	b.term.Print(toJSONString(status))
}

func (b *JSONProgress) error(status interface{}) {
	b.term.Error(toJSONString(status))
}

// Update updates the status lines.
func (b *JSONProgress) Update(total, processed Counter, errors uint, currentFiles map[string]struct{}, start time.Time, secs uint64) {
	status := statusUpdate{
		MessageType:      "status",
		SecondsElapsed:   uint64(time.Since(start) / time.Second),
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

	b.print(status)
}

// ScannerError is the error callback function for the scanner, it prints the
// error in verbose mode and returns nil.
func (b *JSONProgress) ScannerError(item string, err error) error {
	b.error(errorUpdate{
		MessageType: "error",
		Error:       err,
		During:      "scan",
		Item:        item,
	})
	return nil
}

// Error is the error callback function for the archiver, it prints the error and returns nil.
func (b *JSONProgress) Error(item string, err error) error {
	b.error(errorUpdate{
		MessageType: "error",
		Error:       err,
		During:      "archival",
		Item:        item,
	})
	return nil
}

// CompleteItem is the status callback function for the archiver when a
// file/dir has been saved successfully.
func (b *JSONProgress) CompleteItem(messageType, item string, previous, current *restic.Node, s archiver.ItemStats, d time.Duration) {
	if b.v < 2 {
		return
	}

	switch messageType {
	case "dir new":
		b.print(verboseUpdate{
			MessageType:        "verbose_status",
			Action:             "new",
			Item:               item,
			Duration:           d.Seconds(),
			DataSize:           s.DataSize,
			DataSizeInRepo:     s.DataSizeInRepo,
			MetadataSize:       s.TreeSize,
			MetadataSizeInRepo: s.TreeSizeInRepo,
		})
	case "dir unchanged":
		b.print(verboseUpdate{
			MessageType: "verbose_status",
			Action:      "unchanged",
			Item:        item,
		})
	case "dir modified":
		b.print(verboseUpdate{
			MessageType:        "verbose_status",
			Action:             "modified",
			Item:               item,
			Duration:           d.Seconds(),
			DataSize:           s.DataSize,
			DataSizeInRepo:     s.DataSizeInRepo,
			MetadataSize:       s.TreeSize,
			MetadataSizeInRepo: s.TreeSizeInRepo,
		})
	case "file new":
		b.print(verboseUpdate{
			MessageType:    "verbose_status",
			Action:         "new",
			Item:           item,
			Duration:       d.Seconds(),
			DataSize:       s.DataSize,
			DataSizeInRepo: s.DataSizeInRepo,
		})
	case "file unchanged":
		b.print(verboseUpdate{
			MessageType: "verbose_status",
			Action:      "unchanged",
			Item:        item,
		})
	case "file modified":
		b.print(verboseUpdate{
			MessageType:    "verbose_status",
			Action:         "modified",
			Item:           item,
			Duration:       d.Seconds(),
			DataSize:       s.DataSize,
			DataSizeInRepo: s.DataSizeInRepo,
		})
	}
}

// ReportTotal sets the total stats up to now
func (b *JSONProgress) ReportTotal(item string, start time.Time, s archiver.ScanStats) {
	if b.v >= 2 {
		b.print(verboseUpdate{
			MessageType: "verbose_status",
			Action:      "scan_finished",
			Duration:    time.Since(start).Seconds(),
			DataSize:    s.Bytes,
			TotalFiles:  s.Files,
		})
	}
}

// Finish prints the finishing messages.
func (b *JSONProgress) Finish(snapshotID restic.ID, start time.Time, summary *Summary, dryRun bool) {
	b.print(summaryOutput{
		MessageType:         "summary",
		FilesNew:            summary.Files.New,
		FilesChanged:        summary.Files.Changed,
		FilesUnmodified:     summary.Files.Unchanged,
		DirsNew:             summary.Dirs.New,
		DirsChanged:         summary.Dirs.Changed,
		DirsUnmodified:      summary.Dirs.Unchanged,
		DataBlobs:           summary.ItemStats.DataBlobs,
		TreeBlobs:           summary.ItemStats.TreeBlobs,
		DataAdded:           summary.ItemStats.DataSize + summary.ItemStats.TreeSize,
		TotalFilesProcessed: summary.Files.New + summary.Files.Changed + summary.Files.Unchanged,
		TotalBytesProcessed: summary.ProcessedBytes,
		TotalDuration:       time.Since(start).Seconds(),
		SnapshotID:          snapshotID.String(),
		DryRun:              dryRun,
	})
}

// Reset no-op
func (b *JSONProgress) Reset() {
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
	MessageType        string  `json:"message_type"` // "verbose_status"
	Action             string  `json:"action"`
	Item               string  `json:"item"`
	Duration           float64 `json:"duration"` // in seconds
	DataSize           uint64  `json:"data_size"`
	DataSizeInRepo     uint64  `json:"data_size_in_repo"`
	MetadataSize       uint64  `json:"metadata_size"`
	MetadataSizeInRepo uint64  `json:"metadata_size_in_repo"`
	TotalFiles         uint    `json:"total_files"`
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
	DryRun              bool    `json:"dry_run,omitempty"`
}
