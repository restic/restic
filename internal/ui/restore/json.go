package restore

import (
	"time"

	"github.com/restic/restic/internal/ui"
)

type jsonPrinter struct {
	terminal  ui.Terminal
	verbosity uint
}

func NewJSONProgress(terminal ui.Terminal, verbosity uint) ProgressPrinter {
	return &jsonPrinter{
		terminal:  terminal,
		verbosity: verbosity,
	}
}

func (t *jsonPrinter) print(status interface{}) {
	t.terminal.Print(ui.ToJSONString(status))
}

func (t *jsonPrinter) error(status interface{}) {
	t.terminal.Error(ui.ToJSONString(status))
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

func (t *jsonPrinter) Error(item string, err error) error {
	t.error(errorUpdate{
		MessageType: "error",
		Error:       errorObject{err.Error()},
		During:      "restore",
		Item:        item,
	})
	return nil
}

func (t *jsonPrinter) CompleteItem(messageType ItemAction, item string, size uint64) {
	if t.verbosity < 3 {
		return
	}

	var action string
	switch messageType {
	case ActionDirRestored:
		action = "restored"
	case ActionFileRestored:
		action = "restored"
	case ActionOtherRestored:
		action = "restored"
	case ActionFileUpdated:
		action = "updated"
	case ActionFileUnchanged:
		action = "unchanged"
	case ActionDeleted:
		action = "deleted"
	default:
		panic("unknown message type")
	}

	status := verboseUpdate{
		MessageType: "verbose_status",
		Action:      action,
		Item:        item,
		Size:        size,
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

type errorObject struct {
	Message string `json:"message"`
}

type errorUpdate struct {
	MessageType string      `json:"message_type"` // "error"
	Error       errorObject `json:"error"`
	During      string      `json:"during"`
	Item        string      `json:"item"`
}

type verboseUpdate struct {
	MessageType string `json:"message_type"` // "verbose_status"
	Action      string `json:"action"`
	Item        string `json:"item"`
	Size        uint64 `json:"size"`
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
