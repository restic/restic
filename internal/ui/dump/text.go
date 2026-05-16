package dump

import (
	"fmt"
	"time"

	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"
)

type textPrinter struct {
	progress.Printer
	term      ui.Terminal
	verbosity uint
}

// NewTextProgress creates a new text-based progress printer
func NewTextProgress(term ui.Terminal, verbosity uint) ProgressPrinter {
	return &textPrinter{
		Printer:   ui.NewProgressPrinter(false, verbosity, term),
		term:      term,
		verbosity: verbosity,
	}
}

func (t *textPrinter) Update(state State, duration time.Duration) {
	timeElapsed := ui.FormatDuration(duration)
	formattedBytesProcessed := ui.FormatBytes(state.BytesProcessed)

	progress := fmt.Sprintf("[%s] %v files, %v dirs, %v total items %s",
		timeElapsed, state.FilesProcessed, state.DirsProcessed, state.TotalItems, formattedBytesProcessed)

	t.term.SetStatus([]string{progress})
}

func (t *textPrinter) Error(item string, err error) error {
	t.E("ignoring error for %s: %s\n", item, err)
	return nil
}

func (t *textPrinter) CompleteItem(item string, size uint64, nodeType string) {
	t.VV("dumped %s %v with size %v", nodeType, item, ui.FormatBytes(size))
}

func (t *textPrinter) Finish(state State, duration time.Duration) {
	t.term.SetStatus(nil)

	timeElapsed := ui.FormatDuration(duration)
	formattedBytesProcessed := ui.FormatBytes(state.BytesProcessed)

	summary := fmt.Sprintf("Summary: Dumped %d files, %d directories, %d total items (%s) in %s",
		state.FilesProcessed, state.DirsProcessed, state.TotalItems, formattedBytesProcessed, timeElapsed)

	t.term.Print(summary)
}
