package backup

import (
	"fmt"
	"sort"
	"time"

	"github.com/restic/restic/internal/archiver"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/termstatus"
)

// TextProgress reports progress for the `backup` command.
type TextProgress struct {
	*ui.Message

	term *termstatus.Terminal
}

// assert that Backup implements the ProgressPrinter interface
var _ ProgressPrinter = &TextProgress{}

// NewTextProgress returns a new backup progress reporter.
func NewTextProgress(term *termstatus.Terminal, verbosity uint) *TextProgress {
	return &TextProgress{
		Message: ui.NewMessage(term, verbosity),
		term:    term,
	}
}

// Update updates the status lines.
func (b *TextProgress) Update(total, processed Counter, errors uint, currentFiles map[string]struct{}, start time.Time, secs uint64) {
	var status string
	if total.Files == 0 && total.Dirs == 0 {
		// no total count available yet
		status = fmt.Sprintf("[%s] %v files, %s, %d errors",
			ui.FormatDuration(time.Since(start)),
			processed.Files, ui.FormatBytes(processed.Bytes), errors,
		)
	} else {
		var eta, percent string

		if secs > 0 && processed.Bytes < total.Bytes {
			eta = fmt.Sprintf(" ETA %s", ui.FormatSeconds(secs))
			percent = ui.FormatPercent(processed.Bytes, total.Bytes)
			percent += "  "
		}

		// include totals
		status = fmt.Sprintf("[%s] %s%v files %s, total %v files %v, %d errors%s",
			ui.FormatDuration(time.Since(start)),
			percent,
			processed.Files,
			ui.FormatBytes(processed.Bytes),
			total.Files,
			ui.FormatBytes(total.Bytes),
			errors,
			eta,
		)
	}

	lines := make([]string, 0, len(currentFiles)+1)
	for filename := range currentFiles {
		lines = append(lines, filename)
	}
	sort.Strings(lines)
	lines = append([]string{status}, lines...)

	b.term.SetStatus(lines)
}

// ScannerError is the error callback function for the scanner, it prints the
// error in verbose mode and returns nil.
func (b *TextProgress) ScannerError(item string, err error) error {
	b.V("scan: %v\n", err)
	return nil
}

// Error is the error callback function for the archiver, it prints the error and returns nil.
func (b *TextProgress) Error(item string, err error) error {
	b.E("error: %v\n", err)
	return nil
}

// CompleteItem is the status callback function for the archiver when a
// file/dir has been saved successfully.
func (b *TextProgress) CompleteItem(messageType, item string, previous, current *restic.Node, s archiver.ItemStats, d time.Duration) {
	switch messageType {
	case "dir new":
		b.VV("new       %v, saved in %.3fs (%v added, %v stored, %v metadata)",
			item, d.Seconds(), ui.FormatBytes(s.DataSize),
			ui.FormatBytes(s.DataSizeInRepo), ui.FormatBytes(s.TreeSizeInRepo))
	case "dir unchanged":
		b.VV("unchanged %v", item)
	case "dir modified":
		b.VV("modified  %v, saved in %.3fs (%v added, %v stored, %v metadata)",
			item, d.Seconds(), ui.FormatBytes(s.DataSize),
			ui.FormatBytes(s.DataSizeInRepo), ui.FormatBytes(s.TreeSizeInRepo))
	case "file new":
		b.VV("new       %v, saved in %.3fs (%v added)", item,
			d.Seconds(), ui.FormatBytes(s.DataSize))
	case "file unchanged":
		b.VV("unchanged %v", item)
	case "file modified":
		b.VV("modified  %v, saved in %.3fs (%v added, %v stored)", item,
			d.Seconds(), ui.FormatBytes(s.DataSize), ui.FormatBytes(s.DataSizeInRepo))
	}
}

// ReportTotal sets the total stats up to now
func (b *TextProgress) ReportTotal(item string, start time.Time, s archiver.ScanStats) {
	b.V("scan finished in %.3fs: %v files, %s",
		time.Since(start).Seconds(),
		s.Files, ui.FormatBytes(s.Bytes),
	)
}

// Reset status
func (b *TextProgress) Reset() {
	if b.term.CanUpdateStatus() {
		b.term.SetStatus([]string{""})
	}
}

// Finish prints the finishing messages.
func (b *TextProgress) Finish(snapshotID restic.ID, start time.Time, summary *Summary, dryRun bool) {
	b.P("\n")
	b.P("Files:       %5d new, %5d changed, %5d unmodified\n", summary.Files.New, summary.Files.Changed, summary.Files.Unchanged)
	b.P("Dirs:        %5d new, %5d changed, %5d unmodified\n", summary.Dirs.New, summary.Dirs.Changed, summary.Dirs.Unchanged)
	b.V("Data Blobs:  %5d new\n", summary.ItemStats.DataBlobs)
	b.V("Tree Blobs:  %5d new\n", summary.ItemStats.TreeBlobs)
	verb := "Added"
	if dryRun {
		verb = "Would add"
	}
	b.P("%s to the repository: %-5s (%-5s stored)\n", verb,
		ui.FormatBytes(summary.ItemStats.DataSize+summary.ItemStats.TreeSize),
		ui.FormatBytes(summary.ItemStats.DataSizeInRepo+summary.ItemStats.TreeSizeInRepo))
	b.P("\n")
	b.P("processed %v files, %v in %s",
		summary.Files.New+summary.Files.Changed+summary.Files.Unchanged,
		ui.FormatBytes(summary.ProcessedBytes),
		ui.FormatDuration(time.Since(start)),
	)
}
