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

func (t *textPrinter) Update(filesFinished, filesTotal, allBytesWritten, allBytesTotal uint64, duration time.Duration) {
	timeLeft := ui.FormatDuration(duration)
	formattedAllBytesWritten := ui.FormatBytes(allBytesWritten)
	formattedAllBytesTotal := ui.FormatBytes(allBytesTotal)
	allPercent := ui.FormatPercent(allBytesWritten, allBytesTotal)
	progress := fmt.Sprintf("[%s] %s  %v files %s, total %v files %v",
		timeLeft, allPercent, filesFinished, formattedAllBytesWritten, filesTotal, formattedAllBytesTotal)

	t.terminal.SetStatus([]string{progress})
}

func (t *textPrinter) Finish(filesFinished, filesTotal, allBytesWritten, allBytesTotal uint64, duration time.Duration) {
	t.terminal.SetStatus([]string{})

	timeLeft := ui.FormatDuration(duration)
	formattedAllBytesTotal := ui.FormatBytes(allBytesTotal)

	var summary string
	if filesFinished == filesTotal && allBytesWritten == allBytesTotal {
		summary = fmt.Sprintf("Summary: Restored %d Files (%s) in %s", filesTotal, formattedAllBytesTotal, timeLeft)
	} else {
		formattedAllBytesWritten := ui.FormatBytes(allBytesWritten)
		summary = fmt.Sprintf("Summary: Restored %d / %d Files (%s / %s) in %s",
			filesFinished, filesTotal, formattedAllBytesWritten, formattedAllBytesTotal, timeLeft)
	}

	t.terminal.Print(summary)
}
