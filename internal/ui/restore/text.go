package restore

import (
	"fmt"
	"time"

	"github.com/restic/restic/internal/ui"
)

type textPrinter struct {
	terminal  term
	verbosity uint
}

func NewTextProgress(terminal term, verbosity uint) ProgressPrinter {
	return &textPrinter{
		terminal:  terminal,
		verbosity: verbosity,
	}
}

func (t *textPrinter) Update(p State, duration time.Duration) {
	timeLeft := ui.FormatDuration(duration)
	formattedAllBytesWritten := ui.FormatBytes(p.AllBytesWritten)
	formattedAllBytesTotal := ui.FormatBytes(p.AllBytesTotal)
	allPercent := ui.FormatPercent(p.AllBytesWritten, p.AllBytesTotal)
	progress := fmt.Sprintf("[%s] %s  %v files/dirs %s, total %v files/dirs %v",
		timeLeft, allPercent, p.FilesFinished, formattedAllBytesWritten, p.FilesTotal, formattedAllBytesTotal)
	if p.FilesSkipped > 0 {
		progress += fmt.Sprintf(", skipped %v files/dirs %v", p.FilesSkipped, ui.FormatBytes(p.AllBytesSkipped))
	}

	t.terminal.SetStatus([]string{progress})
}

func (t *textPrinter) CompleteItem(messageType ItemAction, item string, size uint64) {
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

	if messageType == ActionDirRestored || messageType == ActionOtherRestored || messageType == ActionDeleted {
		t.terminal.Print(fmt.Sprintf("%-9v %v", action, item))
	} else {
		t.terminal.Print(fmt.Sprintf("%-9v %v with size %v", action, item, ui.FormatBytes(size)))
	}
}

func (t *textPrinter) Finish(p State, duration time.Duration) {
	t.terminal.SetStatus(nil)

	timeLeft := ui.FormatDuration(duration)
	formattedAllBytesTotal := ui.FormatBytes(p.AllBytesTotal)

	var summary string
	if p.FilesFinished == p.FilesTotal && p.AllBytesWritten == p.AllBytesTotal {
		summary = fmt.Sprintf("Summary: Restored %d files/dirs (%s) in %s", p.FilesTotal, formattedAllBytesTotal, timeLeft)
	} else {
		formattedAllBytesWritten := ui.FormatBytes(p.AllBytesWritten)
		summary = fmt.Sprintf("Summary: Restored %d / %d files/dirs (%s / %s) in %s",
			p.FilesFinished, p.FilesTotal, formattedAllBytesWritten, formattedAllBytesTotal, timeLeft)
	}
	if p.FilesSkipped > 0 {
		summary += fmt.Sprintf(", skipped %v files/dirs %v", p.FilesSkipped, ui.FormatBytes(p.AllBytesSkipped))
	}

	t.terminal.Print(summary)
}
