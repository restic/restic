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
	totalFiles                   uint
	totalBytes                   uint64
	restoredFiles, verifiedFiles ui.CounterTo
	restoredBytes, verifiedBytes ui.CounterTo

	// only count hardlinks and special files
	totalHardlinks, totalSpecialFiles uint

	restoredMetadata ui.CounterTo
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
	p.ui.Update(func() { p.restoredBytes.Add(size) })
}

// call when a file blob is verified
func (p *progressUI) completeVerifyBlob(size uint) {
	p.ui.Update(func() { p.verifiedBytes.Add(size) })
}

// call when a file is verified
func (p *progressUI) completeVerifyFile() {
	p.ui.Update(func() { p.verifiedFiles.Add(1) })
}

// call when a file is restored
func (p *progressUI) completeFile() {
	p.ui.Update(func() { p.restoredFiles.Add(1) })
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
	p.ui.Update(func() { p.restoredMetadata.Add(1) })
}

// announce start of file listing phase
func (p *progressUI) startFileListing() {
	start := time.Now()
	setup := func() {}
	progress := func() string {
		progress := fmt.Sprintf("%d directories, %s %d files",
			p.totalDirs,
			ui.FormatBytes(p.totalBytes),
			p.totalFiles)
		return progress
	}
	subtotal := func() string {
		summary := fmt.Sprintf("Created %d directories, listed %s in %d files in %s",
			p.totalDirs,
			ui.FormatBytes(p.totalBytes),
			p.totalFiles,
			ui.FormatDurationSince(start))
		return summary
	}

	p.ui.Set("Creating directories and listing files...", setup, nil, progress, subtotal)
}

// announce start of file content download phase
func (p *progressUI) startFileContent() {
	start := time.Now()
	setup := func() {
		p.restoredFiles = ui.StartCountTo(start, uint64(p.totalFiles))
		p.restoredBytes = ui.StartCountTo(start, p.totalBytes)
	}
	eta := func() time.Duration {
		return p.restoredBytes.ETA(time.Now())
	}
	progress := func() string {
		progress := fmt.Sprintf("%s %s / %s %d / %d files",
			p.restoredBytes.FormatPercent(),
			ui.FormatBytes(uint64(p.restoredBytes.Value())),
			ui.FormatBytes(p.totalBytes),
			p.restoredFiles.Value(),
			p.totalFiles)
		return progress
	}
	subtotal := func() string {
		summary := fmt.Sprintf("Restored %s in %d files in %s",
			ui.FormatBytes(uint64(p.restoredBytes.Value())),
			p.restoredFiles.Value(),
			ui.FormatDurationSince(start))
		return summary
	}
	p.ui.Set("Restoring files content...", setup, eta, progress, subtotal)
}

// announce filesystem metadata restoration phase
func (p *progressUI) startMetadata() {
	start := time.Now()
	setup := func() {
		p.restoredMetadata = ui.StartCountTo(start, uint64(p.totalDirs+p.totalFiles+p.totalHardlinks+p.totalSpecialFiles))
	}
	eta := func() time.Duration {
		return p.restoredMetadata.ETA(time.Now())
	}
	progress := func() string {
		progress := fmt.Sprintf("%s %d / %d timestamps",
			p.restoredMetadata.FormatPercent(),
			p.restoredMetadata.Value(),
			p.restoredMetadata.Target())
		return progress
	}
	subtotal := func() string {
		summary := fmt.Sprintf("Restored %d filesystem timestamps and other metadata in %s",
			p.restoredMetadata.Value(),
			ui.FormatDurationSince(start))
		return summary
	}
	p.ui.Set("Restoring filesystem timestamps and other metadata...", setup, eta, progress, subtotal)
}

// announce start of file content verification phase
func (p *progressUI) startVerify() {
	start := time.Now()
	setup := func() {
		p.verifiedFiles = ui.StartCountTo(start, uint64(p.totalFiles))
		p.verifiedBytes = ui.StartCountTo(start, p.totalBytes)
	}
	eta := func() time.Duration {
		return p.verifiedBytes.ETA(time.Now())
	}
	progress := func() string {
		progress := fmt.Sprintf("%s %s / %s %d / %d files",
			p.verifiedBytes.FormatPercent(),
			ui.FormatBytes(p.verifiedBytes.Value()),
			ui.FormatBytes(p.totalBytes),
			p.verifiedFiles.Value(),
			p.totalFiles)
		return progress
	}
	subtotal := func() string {
		summary := fmt.Sprintf("Verified %s in %d files in %s",
			ui.FormatBytes(p.verifiedBytes.Value()),
			p.verifiedFiles.Value(),
			ui.FormatDurationSince(start))
		return summary
	}
	p.ui.Set("Verifying files content...", setup, eta, progress, subtotal)
}

// show restore summary and statistics
func (p *progressUI) done() {
	// TODO decide if this is useful
	// individual subtotals already provide good idea of the work done
	// summary := func() []string {
	// }
	// p.ui.Set("", nil, summary)
}
