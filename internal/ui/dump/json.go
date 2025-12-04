package dump

import (
	"time"

	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"
)

type jsonPrinter struct {
	progress.Printer
	term      ui.Terminal
	verbosity uint
}

// NewJSONProgress creates a new JSON-based progress printer
func NewJSONProgress(term ui.Terminal, verbosity uint) ProgressPrinter {
	return &jsonPrinter{
		Printer:   ui.NewProgressPrinter(true, verbosity, term),
		term:      term,
		verbosity: verbosity,
	}
}

func (t *jsonPrinter) print(status interface{}) {
	t.term.Print(ui.ToJSONString(status))
}

func (t *jsonPrinter) error(status interface{}) {
	t.term.Error(ui.ToJSONString(status))
}

func (t *jsonPrinter) Update(state State, duration time.Duration) {
	status := statusUpdate{
		MessageType:    "status",
		SecondsElapsed: uint64(duration / time.Second),
		FilesProcessed: state.FilesProcessed,
		DirsProcessed:  state.DirsProcessed,
		TotalItems:     state.TotalItems,
		BytesProcessed: state.BytesProcessed,
	}

	t.print(status)
}

func (t *jsonPrinter) Error(item string, err error) error {
	t.error(errorUpdate{
		MessageType: "error",
		Error:       errorObject{err.Error()},
		During:      "dump",
		Item:        item,
	})
	return nil
}

func (t *jsonPrinter) CompleteItem(item string, size uint64, nodeType string) {
	if t.verbosity < 3 {
		return
	}

	status := verboseUpdate{
		MessageType: "verbose_status",
		Action:      "dumped",
		NodeType:    nodeType,
		Item:        item,
		Size:        size,
	}
	t.print(status)
}

func (t *jsonPrinter) Finish(state State, duration time.Duration) {
	status := summaryOutput{
		MessageType:    "summary",
		SecondsElapsed: uint64(duration / time.Second),
		FilesProcessed: state.FilesProcessed,
		DirsProcessed:  state.DirsProcessed,
		TotalItems:     state.TotalItems,
		BytesProcessed: state.BytesProcessed,
	}
	t.print(status)
}

type statusUpdate struct {
	MessageType    string `json:"message_type"` // "status"
	SecondsElapsed uint64 `json:"seconds_elapsed,omitempty"`
	FilesProcessed uint64 `json:"files_processed,omitempty"`
	DirsProcessed  uint64 `json:"dirs_processed,omitempty"`
	TotalItems     uint64 `json:"total_items,omitempty"`
	BytesProcessed uint64 `json:"bytes_processed,omitempty"`
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
	NodeType    string `json:"node_type"`
	Item        string `json:"item"`
	Size        uint64 `json:"size"`
}

type summaryOutput struct {
	MessageType    string `json:"message_type"` // "summary"
	SecondsElapsed uint64 `json:"seconds_elapsed,omitempty"`
	FilesProcessed uint64 `json:"files_processed,omitempty"`
	DirsProcessed  uint64 `json:"dirs_processed,omitempty"`
	TotalItems     uint64 `json:"total_items,omitempty"`
	BytesProcessed uint64 `json:"bytes_processed,omitempty"`
}
