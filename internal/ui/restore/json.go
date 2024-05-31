package restore

import (
	"time"

	"github.com/restic/restic/internal/ui"
)

type jsonPrinter struct {
	terminal term
}

func NewJSONProgress(terminal term) ProgressPrinter {
	return &jsonPrinter{
		terminal: terminal,
	}
}

func (t *jsonPrinter) print(status interface{}) {
	t.terminal.Print(ui.ToJSONString(status))
}

func (t *jsonPrinter) Update(p State, duration time.Duration) {
	status := statusUpdate{
		MessageType:    "status",
		SecondsElapsed: uint64(duration / time.Second),
		TotalFiles:     p.FilesTotal,
		FilesRestored:  p.FilesFinished,
		FilesSkipped:   p.FilesSkipped,
		TotalBytes:     p.AllBytesTotal,
		BytesRestored:  p.AllBytesWritten,
		BytesSkipped:   p.AllBytesSkipped,
	}

	if p.AllBytesTotal > 0 {
		status.PercentDone = float64(p.AllBytesWritten) / float64(p.AllBytesTotal)
	}

	t.print(status)
}

func (t *jsonPrinter) Finish(p State, duration time.Duration) {
	status := summaryOutput{
		MessageType:    "summary",
		SecondsElapsed: uint64(duration / time.Second),
		TotalFiles:     p.FilesTotal,
		FilesRestored:  p.FilesFinished,
		FilesSkipped:   p.FilesSkipped,
		TotalBytes:     p.AllBytesTotal,
		BytesRestored:  p.AllBytesWritten,
		BytesSkipped:   p.AllBytesSkipped,
	}
	t.print(status)
}

type statusUpdate struct {
	MessageType    string  `json:"message_type"` // "status"
	SecondsElapsed uint64  `json:"seconds_elapsed,omitempty"`
	PercentDone    float64 `json:"percent_done"`
	TotalFiles     uint64  `json:"total_files,omitempty"`
	FilesRestored  uint64  `json:"files_restored,omitempty"`
	FilesSkipped   uint64  `json:"files_skipped,omitempty"`
	TotalBytes     uint64  `json:"total_bytes,omitempty"`
	BytesRestored  uint64  `json:"bytes_restored,omitempty"`
	BytesSkipped   uint64  `json:"bytes_skipped,omitempty"`
}

type summaryOutput struct {
	MessageType    string `json:"message_type"` // "summary"
	SecondsElapsed uint64 `json:"seconds_elapsed,omitempty"`
	TotalFiles     uint64 `json:"total_files,omitempty"`
	FilesRestored  uint64 `json:"files_restored,omitempty"`
	FilesSkipped   uint64 `json:"files_skipped,omitempty"`
	TotalBytes     uint64 `json:"total_bytes,omitempty"`
	BytesRestored  uint64 `json:"bytes_restored,omitempty"`
	BytesSkipped   uint64 `json:"bytes_skipped,omitempty"`
}
