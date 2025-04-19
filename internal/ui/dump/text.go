package dump

import (
	"fmt"
	"time"

	"github.com/restic/restic/internal/ui"
)

type textPrinter struct {
	*ui.Message

	terminal ui.Terminal
}

// NewTextProgress creates a new text-based progress printer
func NewTextProgress(terminal ui.Terminal, verbosity uint) ProgressPrinter {
	return &textPrinter{
		Message:  ui.NewMessage(terminal, verbosity),
		terminal: terminal,
	}
}

func (t *textPrinter) Update(state State, duration time.Duration) {
	timeElapsed := ui.FormatDuration(duration)
	formattedBytesProcessed := ui.FormatBytes(state.BytesProcessed)
	
	progress := fmt.Sprintf("[%s] %v files, %v dirs, %v total items %s",
		timeElapsed, state.FilesProcessed, state.DirsProcessed, state.TotalItems, formattedBytesProcessed)

	t.terminal.SetStatus([]string{progress})
}

func (t *textPrinter) Error(item string, err error) error {
	t.E("ignoring error for %s: %s\n", item, err)
	return nil
}

func (t *textPrinter) CompleteItem(item string, size uint64, nodeType string) {
	t.VV("dumped %s %v with size %v", nodeType, item, ui.FormatBytes(size))
}

func (t *textPrinter) Finish(state State, duration time.Duration) {
	t.terminal.SetStatus(nil)

	timeElapsed := ui.FormatDuration(duration)
	formattedBytesProcessed := ui.FormatBytes(state.BytesProcessed)

	summary := fmt.Sprintf("Summary: Dumped %d files, %d directories, %d total items (%s) in %s",
		state.FilesProcessed, state.DirsProcessed, state.TotalItems, formattedBytesProcessed, timeElapsed)

	t.terminal.Print(summary)
}
