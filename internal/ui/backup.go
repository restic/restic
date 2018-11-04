package ui

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/restic/restic/internal/archiver"
	"github.com/restic/restic/internal/restic"
)

// Backup reports progress for the `backup` command.
type Backup struct {
	ui ProgressUI

	dirs, files, others CounterTo
	bytes               CounterTo
	errors              uint

	currentFiles map[string]struct{}

	summary struct {
		Files, Dirs struct {
			New       uint
			Changed   uint
			Unchanged uint
		}
		archiver.ItemStats
	}
}

// NewBackup returns a new backup progress reporter.
func NewBackup(ui ProgressUI) *Backup {
	b := &Backup{
		ui:           ui,
		currentFiles: make(map[string]struct{}),
	}

	progress := func() string {
		return fmt.Sprintf("Backing up: %v files %s, total %v files %v, %d errors",
			b.files.Current,
			FormatBytes(b.bytes.Current),
			b.files.Total,
			FormatBytes(b.bytes.Total),
			b.errors)
	}

	status := func() []string {
		lines := make([]string, 0, len(b.currentFiles)+1)
		for filename := range b.currentFiles {
			lines = append(lines, filename)
		}
		sort.Sort(sort.StringSlice(lines))

		return lines
	}

	summary := func(duration time.Duration) {
		b.ui.P("\n")
		b.ui.P("Files:       %5d new, %5d changed, %5d unmodified\n", b.summary.Files.New, b.summary.Files.Changed, b.summary.Files.Unchanged)
		b.ui.P("Dirs:        %5d new, %5d changed, %5d unmodified\n", b.summary.Dirs.New, b.summary.Dirs.Changed, b.summary.Dirs.Unchanged)
		b.ui.V("Data Blobs:  %5d new\n", b.summary.ItemStats.DataBlobs)
		b.ui.V("Tree Blobs:  %5d new\n", b.summary.ItemStats.TreeBlobs)
		b.ui.P("Added to the repo: %-5s\n", FormatBytes(int64(b.summary.ItemStats.DataSize+b.summary.ItemStats.TreeSize)))
		b.ui.P("\n")
		b.ui.P("processed %v files, %v in %s",
			b.summary.Files.New+b.summary.Files.Changed+b.summary.Files.Unchanged,
			FormatBytes(b.bytes.Current),
			formatDuration(duration),
		)
	}

	ui.StartPhase(progress, status, b.bytes.Percent, summary)

	return b
}

// ScannerError is the error callback function for the scanner, it prints the
// error in verbose mode and returns nil.
func (b *Backup) ScannerError(item string, fi os.FileInfo, err error) error {
	b.ui.V("scan: %v\n", err)
	return nil
}

// Error is the error callback function for the archiver, it prints the error and returns nil.
func (b *Backup) Error(item string, fi os.FileInfo, err error) error {
	b.ui.E("error: %v\n", err)
	b.ui.Update(func() { b.errors++ })
	return nil
}

// StartFile is called when a file is being processed by a worker.
func (b *Backup) StartFile(filename string) {
	b.ui.Update(func() { b.currentFiles[filename] = struct{}{} })
}

// CompleteBlob is called for all saved blobs for files.
func (b *Backup) CompleteBlob(filename string, bytes uint64) {
	b.ui.Update(func() { b.bytes.Current += int64(bytes) })
}

// CompleteItemFn is the status callback function for the archiver when a
// file/dir has been saved successfully.
func (b *Backup) CompleteItemFn(item string, previous, current *restic.Node, s archiver.ItemStats, d time.Duration) {
	filedone := func() { delete(b.currentFiles, item) }

	if current == nil {
		// error occurred, tell the status display to remove the line
		b.ui.Update(filedone)
		return
	}

	if current.Type == "dir" {
		var summary *uint

		switch {
		case previous == nil:
			b.ui.VV("new       %v, saved in %.3fs (%v added, %v metadata)", item, d.Seconds(), FormatBytes(int64(s.DataSize)), FormatBytes(int64(s.TreeSize)))
			summary = &b.summary.Dirs.New

		case previous.Equals(*current):
			b.ui.VV("unchanged %v", item)
			summary = &b.summary.Dirs.Unchanged

		default:
			b.ui.VV("modified  %v, saved in %.3fs (%v added, %v metadata)", item, d.Seconds(), FormatBytes(int64(s.DataSize)), FormatBytes(int64(s.TreeSize)))
			summary = &b.summary.Dirs.Changed
		}

		b.ui.Update(func() {
			b.dirs.Current++
			*summary++
			b.summary.ItemStats.Add(s)
		})
	} else if current.Type == "file" {
		var summary *uint

		switch {
		case previous == nil:
			b.ui.VV("new       %v, saved in %.3fs (%v added)", item, d.Seconds(), FormatBytes(int64(s.DataSize)))
			summary = &b.summary.Files.New

		case previous.Equals(*current):
			b.ui.VV("unchanged %v", item)
			summary = &b.summary.Files.Unchanged

		default:
			b.ui.VV("modified  %v, saved in %.3fs (%v added)", item, d.Seconds(), FormatBytes(int64(s.DataSize)))
			summary = &b.summary.Files.Changed
		}

		b.ui.Update(func() {
			b.files.Current++
			*summary++
			b.summary.ItemStats.Add(s)
			filedone()
		})
	}

}

// ReportTotal sets the total stats up to now
func (b *Backup) ReportTotal(item string, s archiver.ScanStats) {
	b.ui.Update(func() {
		b.dirs.Total = int64(s.Dirs)
		b.files.Total = int64(s.Files)
		b.others.Total = int64(s.Others)
		b.bytes.Total = int64(s.Bytes)
	})

	if item == "" {
		// b.ui.V("scan finished in %.3fs: %v files, %s",
		// 	time.Since(b.start).Seconds(),
		// 	s.Files, FormatBytes(s.Bytes),
		// )
	}
}
