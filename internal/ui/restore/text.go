package restore

import (
	"fmt"
	"time"

	"github.com/restic/restic/internal/ui"
)

type textPrinter struct {
	terminal term
}

func NewTextProgress(terminal term) ProgressPrinter {
	return &textPrinter{
		terminal: terminal,
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

func (t *textPrinter) Finish(p State, duration time.Duration) {
	t.terminal.SetStatus([]string{})

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
