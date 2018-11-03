package ui

import (
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
	errors              Counter

	currentFiles map[string]struct{}

	summary struct {
		Files, Dirs struct {
			New       Counter
			Changed   Counter
			Unchanged Counter
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

	metrics := map[string]interface{}{
		"dirs":    &b.dirs,
		"files":   &b.files,
		"bytes":   &b.bytes,
		"percent": &b.bytes,
		"errors":  &b.errors,
		"summary": &b.summary,
	}

	setup := func() {}

	progress := `{{.files.Value}} files {{.bytes.FormatBytes}}, total {{.files.Target.Value}} files {{.bytes.Target.FormatBytes}}, {{.errors.Value}} errors`
	summary := `
Files:       {{printf "%5d" .summary.Files.New.Value}} new, {{printf "%5d" .summary.Files.Changed.Value}} changed, {{printf "%5d" .summary.Files.Unchanged.Value}} unmodified
Dirs:        {{printf "%5d" .summary.Dirs.New.Value}} new, {{printf "%5d" .summary.Dirs.Changed.Value}} changed, {{printf "%5d" .summary.Dirs.Unchanged.Value}} unmodified
Data Blobs:  {{printf "%5d" .summary.ItemStats.DataBlobs}} new
Tree Blobs:  {{printf "%5d" .summary.ItemStats.TreeBlobs}} new
Added to the repo: {{ FormatBytes (.summary.ItemStats.DataSize + .summary.ItemStats.TreeSize) | print "%-5s" }}

processed %v files, {{.bytes.FormatBytes}} in %s
	`

	// b.P("\n")
	// b.P("Files:       %5d new, %5d changed, %5d unmodified\n", b.summary.Files.New, b.summary.Files.Changed, b.summary.Files.Unchanged)
	// b.P("Dirs:        %5d new, %5d changed, %5d unmodified\n", b.summary.Dirs.New, b.summary.Dirs.Changed, b.summary.Dirs.Unchanged)
	// b.V("Data Blobs:  %5d new\n", b.summary.ItemStats.DataBlobs)
	// b.V("Tree Blobs:  %5d new\n", b.summary.ItemStats.TreeBlobs)
	// b.P("Added to the repo: %-5s\n", FormatBytes(b.summary.ItemStats.DataSize+b.summary.ItemStats.TreeSize))
	// b.P("\n")
	// b.P("processed %v files, %v in %s",
	// 	b.summary.Files.New+b.summary.Files.Changed+b.summary.Files.Unchanged,
	// 	FormatBytes(b.totalBytes),
	// 	formatDuration(time.Since(b.start)),
	// )

	status := func() []string {
		// 	lines := make([]string, 0, len(currentFiles)+1)
		// 	for filename := range currentFiles {
		// 		lines = append(lines, filename)
		// 	}
		// 	sort.Sort(sort.StringSlice(lines))
		// 	lines = append([]string{status}, lines...)

		lines := make([]string, 0, len(b.currentFiles)+1)
		for filename := range b.currentFiles {
			lines = append(lines, filename)
		}
		sort.Sort(sort.StringSlice(lines))

		return lines
	}

	ui.Set("backup", setup, metrics, progress, status, summary)

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
	b.ui.Update(func() { b.errors.Add(1) })
	return nil
}

// StartFile is called when a file is being processed by a worker.
func (b *Backup) StartFile(filename string) {
	b.ui.Update(func() { b.currentFiles[filename] = struct{}{} })
}

// CompleteBlob is called for all saved blobs for files.
func (b *Backup) CompleteBlob(filename string, bytes uint64) {
	b.ui.Update(func() { b.bytes.Add(int64(bytes)) })
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
		var summary *Counter

		switch {
		case previous == nil:
			b.ui.VV("new       %v, saved in %.3fs (%v added, %v metadata)", item, d.Seconds(), FormatBytes(s.DataSize), FormatBytes(s.TreeSize))
			summary = &b.summary.Dirs.New

		case previous.Equals(*current):
			b.ui.VV("unchanged %v", item)
			summary = &b.summary.Dirs.Unchanged

		default:
			b.ui.VV("modified  %v, saved in %.3fs (%v added, %v metadata)", item, d.Seconds(), FormatBytes(s.DataSize), FormatBytes(s.TreeSize))
			summary = &b.summary.Dirs.Changed
		}

		b.ui.Update(func() {
			b.dirs.Counter.Add(1)
			summary.Add(1)
			b.summary.ItemStats.Add(s)
		})
	} else if current.Type == "file" {
		var summary *Counter

		switch {
		case previous == nil:
			b.ui.VV("new       %v, saved in %.3fs (%v added)", item, d.Seconds(), FormatBytes(s.DataSize))
			summary = &b.summary.Files.New

		case previous.Equals(*current):
			b.ui.VV("unchanged %v", item)
			summary = &b.summary.Files.Unchanged

		default:
			b.ui.VV("modified  %v, saved in %.3fs (%v added)", item, d.Seconds(), FormatBytes(s.DataSize))
			summary = &b.summary.Files.Changed
		}

		b.ui.Update(func() {
			b.files.Counter.Add(1)
			summary.Add(1)
			b.summary.ItemStats.Add(s)
			filedone()
		})
	}

}

// ReportTotal sets the total stats up to now
func (b *Backup) ReportTotal(item string, s archiver.ScanStats) {
	b.ui.Update(func() {
		b.dirs.Target().Set(int64(s.Dirs))
		b.files.Target().Set(int64(s.Files))
		b.others.Target().Set(int64(s.Others))
		b.bytes.Target().Set(int64(s.Bytes))
	})

	if item == "" {
		// b.ui.V("scan finished in %.3fs: %v files, %s",
		// 	time.Since(b.start).Seconds(),
		// 	s.Files, FormatBytes(s.Bytes),
		// )
	}
}
