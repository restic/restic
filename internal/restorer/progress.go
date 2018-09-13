package restorer

import (
	"fmt"
	"time"

	"github.com/restic/restic/internal/ui"
)

// progressUI reports progress for the `restore` command.
// Assumes the following multiphase restore implementation:
// 1. precreate directories and select files for restore
// 2. restore file content
// 3. create special files and restore filesystem metadata
// 4. optionally, verify file contents
type progressUI struct {
	ui ui.ProgressUI

	// directories are eagerly created
	totalDirs uint

	// files are selected first, then restored, then optionally verified
	totalFiles, restoredFiles, verifiedFiles uint
	totalBytes, restoredBytes, verifiedBytes uint64

	// only count hardlinks and special files
	totalHardlinks, totalSpecialFiles uint

	restoredMetadata uint
}

func newProgressUI(ui ui.ProgressUI) *progressUI {
	return &progressUI{ui: ui}
}

// call when a directory is created
func (p *progressUI) addDir() {
	p.ui.Update(func() { p.totalDirs++ })
}

// call when a file is selected
func (p *progressUI) addFile(size uint64) {
	p.ui.Update(func() {
		p.totalFiles++
		p.totalBytes += size
	})
}

// call when a hardlink is selected
func (p *progressUI) addHardlink() {
	p.ui.Update(func() { p.totalHardlinks++ })
}

// call when a special file is selected
func (p *progressUI) addSpecialFile() {
	p.ui.Update(func() { p.totalSpecialFiles++ })
}

// call when a file blob is written to a file
func (p *progressUI) completeBlob(size uint) {
	p.ui.Update(func() { p.restoredBytes += uint64(size) })
}

// call when a file blob is verified
func (p *progressUI) completeVerifyBlob(size uint) {
	p.ui.Update(func() { p.verifiedBytes += uint64(size) })
}

// call when a file is verified
func (p *progressUI) completeVerifyFile() {
	p.ui.Update(func() { p.verifiedFiles++ })
}

// call when a file is restored
func (p *progressUI) completeFile() {
	p.ui.Update(func() { p.restoredFiles++ })
}

// call when a hardlink is created
func (p *progressUI) completeHardlink() {
	// do nothing, hardlink creation progress is not shown
}

// call when a special file is created
func (p *progressUI) completeSpecialFile() {
	// do nothing, special file creation progress is not shown
}

// call when file/directory filesystem metadata is restored
func (p *progressUI) completeMetadata() {
	p.ui.Update(func() { p.restoredMetadata++ })
}

// announce start of file listing phase
func (p *progressUI) startFileListing() {
	start := time.Now()
	progress := func() []string {
		progress := fmt.Sprintf("%d directories, %s %d files",
			p.totalDirs,
			ui.FormatBytes(p.totalBytes),
			p.totalFiles)
		return []string{progress}
	}
	subtotal := func() []string {
		summary := fmt.Sprintf("Created %d directories, listed %s in %d files in %s",
			p.totalDirs,
			ui.FormatBytes(p.totalBytes),
			p.totalFiles,
			ui.FormatDurationSince(start))
		return []string{summary}
	}

	p.ui.Set("Creating directories and listing files...", progress, subtotal)
}

// announce start of file content download phase
func (p *progressUI) startFileContent() {
	start := time.Now()
	progress := func() []string {
		progress := fmt.Sprintf("%s %s / %s %d / %d files",
			ui.FormatPercent(p.restoredBytes, p.totalBytes),
			ui.FormatBytes(p.restoredBytes),
			ui.FormatBytes(p.totalBytes),
			p.restoredFiles,
			p.totalFiles)
		return []string{progress}
	}
	subtotal := func() []string {
		summary := fmt.Sprintf("Restored %s in %d files in %s",
			ui.FormatBytes(p.restoredBytes),
			p.restoredFiles,
			ui.FormatDurationSince(start))
		return []string{summary}
	}
	p.ui.Set("Restoring files content...", progress, subtotal)
}

// announce filesystem metadata restoration phase
func (p *progressUI) startMetadata() {
	start := time.Now()
	progress := func() []string {
		totalMetadata := p.totalDirs + p.totalFiles + p.totalHardlinks + p.totalSpecialFiles
		progress := fmt.Sprintf("%s %d / %d timestamps",
			ui.FormatPercent(uint64(p.restoredMetadata), uint64(totalMetadata)),
			p.restoredMetadata,
			totalMetadata)
		return []string{progress}
	}
	subtotal := func() []string {
		summary := fmt.Sprintf("Restored %d filesystem timestamps and other metadata in %s",
			p.restoredMetadata,
			ui.FormatDurationSince(start))
		return []string{summary}
	}
	p.ui.Set("Restoring filesystem timestamps and other metadata...", progress, subtotal)
}

// announce start of file content verification phase
func (p *progressUI) startVerify() {
	start := time.Now()
	progress := func() []string {
		progress := fmt.Sprintf("%s %s / %s %d / %d files",
			ui.FormatPercent(p.verifiedBytes, p.totalBytes),
			ui.FormatBytes(p.verifiedBytes),
			ui.FormatBytes(p.totalBytes),
			p.verifiedFiles,
			p.totalFiles)
		return []string{progress}
	}
	subtotal := func() []string {
		summary := fmt.Sprintf("Verified %s in %d files in %s",
			ui.FormatBytes(p.verifiedBytes),
			p.verifiedFiles,
			ui.FormatDurationSince(start))
		return []string{summary}
	}
	p.ui.Set("Verifying files content...", progress, subtotal)
}

// show restore summary and statistics
func (p *progressUI) done() {
	// TODO decide if this is useful
	// individual subtotals already provide good idea of the work done
	// summary := func() []string {
	// }
	// p.ui.Set("", nil, summary)
}
