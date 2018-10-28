package ui

import (
	"os"
	"time"

	"github.com/restic/restic/internal/archiver"
	"github.com/restic/restic/internal/restic"
)

// type fileWorkerMessage struct {
// 	filename string
// 	done     bool
// }

// Backup reports progress for the `backup` command.
type Backup struct {
	ui ProgressUI

	dirs, files, others CounterTo
	bytes               CounterTo
	errors              Counter

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
		ui: ui,
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

	progress := "{{.files.Value}} files {{.bytes.FormatBytes}}, total {{.files.Target.Value}} files {{.bytes.Target.FormatBytes}}, {{.errors.Value}} errors"
	summary := `
Files:       {{.summary.Files.New.Value}} new, {{.summary.Files.Changed.Value}} changed, {{.summary.Files.Unchanged.Value}} unmodified
Dirs:        {{.summary.Dirs.New.Value}} new, {{.summary.Dirs.Changed.Value}} changed, {{.summary.Dirs.Unchanged.Value}} unmodified
Data Blobs:  {{.summary.ItemStats.DataBlobs}} new
Tree Blobs:  {{.summary.ItemStats.TreeBlobs}} new
Added to the repo: %-5s

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

	ui.Set("backup", setup, metrics, progress, summary)

	return b
}

// update updates the status lines.
// func (b *Backup) update(total, processed counter, errors uint, currentFiles map[string]struct{}, secs uint64) {

// 	lines := make([]string, 0, len(currentFiles)+1)
// 	for filename := range currentFiles {
// 		lines = append(lines, filename)
// 	}
// 	sort.Sort(sort.StringSlice(lines))
// 	lines = append([]string{status}, lines...)

// 	b.term.SetStatus(lines)
// }

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
	// b.workerCh <- fileWorkerMessage{
	// 	filename: filename,
	// }
}

// CompleteBlob is called for all saved blobs for files.
func (b *Backup) CompleteBlob(filename string, bytes uint64) {
	b.ui.Update(func() { b.bytes.Add(int64(bytes)) })
}

// CompleteItemFn is the status callback function for the archiver when a
// file/dir has been saved successfully.
func (b *Backup) CompleteItemFn(item string, previous, current *restic.Node, s archiver.ItemStats, d time.Duration) {
	if current == nil {
		// error occurred, tell the status display to remove the line
		// b.workerCh <- fileWorkerMessage{
		// 	filename: item,
		// 	done:     true,
		// }
		return
	}

	var progress, summary *Counter

	switch current.Type {
	case "file":
		progress = &b.files.Counter
		// b.workerCh <- fileWorkerMessage{
		// 	filename: item,
		// 	done:     true,
		// }
	case "dir":
		progress = &b.dirs.Counter
	}

	if current.Type == "dir" {
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
	} else if current.Type == "file" {

		// b.workerCh <- fileWorkerMessage{
		// 	done:     true,
		// 	filename: item,
		// }

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
	}

	b.ui.Update(func() {
		progress.Add(1)
		summary.Add(1)
		b.summary.ItemStats.Add(s)
	})
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
