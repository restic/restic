package archiver

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
)

// BackupProgress reports progress for the `backup` command.
type BackupProgress struct {
	ui ui.ProgressUI

	dirs, files, bytes, others struct {
		current, total int64
	}
	errors uint

	currentFiles map[string]struct{}

	summary struct {
		Files, Dirs struct {
			New       uint
			Changed   uint
			Unchanged uint
		}
		ItemStats
	}
}

// NewBackupProgress returns a new backup progress reporter.
func NewBackupProgress(pm ui.ProgressUI) *BackupProgress {
	b := &BackupProgress{
		ui:           pm,
		currentFiles: make(map[string]struct{}),
	}

	// NB backup does not fit perfectly into "sequence of independent phases" ProgressUI model
	// There are actually two overlapping and dependent "phases"
	// - scanning of files to backup, which calculates totals
	// - actual backup
	// We need to time backup phase to calculate ETA, so this is what we model as a phase
	// The scanner runs in parallel and updates totals, but it is not represented in the
	// progress otherwise

	progress := func() string {
		return fmt.Sprintf("Backing up: %v files %s, total %v files %v, %d errors",
			b.files.current,
			ui.FormatBytes(b.bytes.current),
			b.files.total,
			ui.FormatBytes(b.bytes.total),
			b.errors)
	}

	percent := func() (int64, int64) {
		return b.bytes.current, b.bytes.total
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
		b.ui.P("Added to the repo: %-5s\n", ui.FormatBytes(int64(b.summary.ItemStats.DataSize+b.summary.ItemStats.TreeSize)))
		b.ui.P("\n")
		b.ui.P("processed %v files, %v in %s",
			b.summary.Files.New+b.summary.Files.Changed+b.summary.Files.Unchanged,
			ui.FormatBytes(b.bytes.current),
			ui.FormatDuration(duration),
		)
	}

	pm.StartPhase(progress, status, percent, summary)

	return b
}

// ScannerError is the error callback function for the scanner, it prints the
// error in verbose mode and returns nil.
func (b *BackupProgress) ScannerError(item string, fi os.FileInfo, err error) error {
	b.ui.V("scan: %v\n", err)
	return nil
}

// Error is the error callback function for the archiver, it prints the error and returns nil.
func (b *BackupProgress) Error(item string, fi os.FileInfo, err error) error {
	b.ui.E("error: %v\n", err)
	b.ui.Update(func() { b.errors++ })
	return nil
}

// StartFile is called when a file is being processed by a worker.
func (b *BackupProgress) StartFile(filename string) {
	b.ui.Update(func() { b.currentFiles[filename] = struct{}{} })
}

// CompleteBlob is called for all saved blobs for files.
func (b *BackupProgress) CompleteBlob(filename string, bytes uint64) {
	b.ui.Update(func() { b.bytes.current += int64(bytes) })
}

// CompleteItemFn is the status callback function for the archiver when a
// file/dir has been saved successfully.
func (b *BackupProgress) CompleteItemFn(item string, previous, current *restic.Node, s ItemStats, d time.Duration) {
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
			b.ui.VV("new       %v, saved in %.3fs (%v added, %v metadata)", item, d.Seconds(), ui.FormatBytes(int64(s.DataSize)), ui.FormatBytes(int64(s.TreeSize)))
			summary = &b.summary.Dirs.New

		case previous.Equals(*current):
			b.ui.VV("unchanged %v", item)
			summary = &b.summary.Dirs.Unchanged

		default:
			b.ui.VV("modified  %v, saved in %.3fs (%v added, %v metadata)", item, d.Seconds(), ui.FormatBytes(int64(s.DataSize)), ui.FormatBytes(int64(s.TreeSize)))
			summary = &b.summary.Dirs.Changed
		}

		b.ui.Update(func() {
			b.dirs.current++
			*summary++
			b.summary.ItemStats.Add(s)
		})
	} else if current.Type == "file" {
		var summary *uint

		switch {
		case previous == nil:
			b.ui.VV("new       %v, saved in %.3fs (%v added)", item, d.Seconds(), ui.FormatBytes(int64(s.DataSize)))
			summary = &b.summary.Files.New

		case previous.Equals(*current):
			b.ui.VV("unchanged %v", item)
			summary = &b.summary.Files.Unchanged

		default:
			b.ui.VV("modified  %v, saved in %.3fs (%v added)", item, d.Seconds(), ui.FormatBytes(int64(s.DataSize)))
			summary = &b.summary.Files.Changed
		}

		b.ui.Update(func() {
			b.files.current++
			*summary++
			b.summary.ItemStats.Add(s)
			filedone()
		})
	}

}

// ReportTotal sets the total stats up to now
func (b *BackupProgress) ReportTotal(item string, s ScanStats) {
	b.ui.Update(func() {
		b.dirs.total = int64(s.Dirs)
		b.files.total = int64(s.Files)
		b.others.total = int64(s.Others)
		b.bytes.total = int64(s.Bytes)
	})

	if item == "" {
		// b.ui.V("scan finished in %.3fs: %v files, %s",
		// 	time.Since(b.start).Seconds(),
		// 	s.Files, FormatBytes(s.Bytes),
		// )
	}
}
