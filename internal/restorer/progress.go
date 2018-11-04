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
	dirs int64

	// files are selected first, then restored, then optionally verified
	files struct {
		total    int64
		restored int64
		verified int64
	}
	bytes struct {
		total    int64
		restored int64
		verified int64
	}

	// TODO track downloaded bytes

	// only count hardlinks and special files
	hardlinks, specialfiles int64

	metadata int64
}

func newProgressUI(ui ui.ProgressUI) *progressUI {
	return &progressUI{ui: ui}
}

// call when a directory is created
func (p *progressUI) addDir() {
	p.ui.Update(func() { p.dirs++ })
}

// call when a file is selected
func (p *progressUI) addFile(size uint64) {
	p.ui.Update(func() {
		p.files.total++
		p.bytes.total += int64(size)
	})
}

// call when a hardlink is selected
func (p *progressUI) addHardlink() {
	p.ui.Update(func() { p.hardlinks++ })
}

// call when a special file is selected
func (p *progressUI) addSpecialFile() {
	p.ui.Update(func() { p.specialfiles++ })
}

// call when a file blob is written to a file
func (p *progressUI) completeBlob(size uint) {
	p.ui.Update(func() { p.bytes.restored += int64(size) })
}

// call when a file blob is verified
func (p *progressUI) completeVerifyBlob(size uint) {
	p.ui.Update(func() { p.bytes.verified += int64(size) })
}

// call when a file is verified
func (p *progressUI) completeVerifyFile() {
	p.ui.Update(func() { p.files.verified++ })
}

// call when a file is restored
func (p *progressUI) completeFile() {
	p.ui.Update(func() { p.files.restored++ })
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
	p.ui.Update(func() { p.metadata++ })
}

// announce start of file listing phase
func (p *progressUI) startFileListing() {
	progress := func() string {
		return fmt.Sprintf("Creating directories and listing files: %d directories, %s in %d files",
			p.dirs,
			ui.FormatBytes(p.bytes.total),
			p.files.total,
		)
	}

	summary := func(time.Duration) {
		p.ui.P("Created %d directories, listed %s in %d files",
			p.dirs,
			ui.FormatBytes(p.bytes.total),
			p.files.total,
		)
	}

	p.ui.StartPhase(progress, nil, nil, summary)
}

// announce start of file content download phase
func (p *progressUI) startFileContent() {
	progress := func() string {
		return fmt.Sprintf("Restoring files content: %s / %s %d / %d files",
			ui.FormatBytes(p.bytes.restored),
			ui.FormatBytes(p.bytes.total),
			p.files.restored,
			p.files.total,
		)
	}
	percent := func() (int64, int64) {
		return p.bytes.restored, p.bytes.total
	}
	summary := func(time.Duration) {
		// TODO speed
		p.ui.P("Restored %s in %d files",
			ui.FormatBytes(p.bytes.restored),
			p.files.restored,
		)
	}
	p.ui.StartPhase(progress, nil, percent, summary)
}

// announce filesystem metadata restoration phase
func (p *progressUI) startMetadata() {
	metadataTotal := func() int64 {
		return p.dirs + p.files.total + p.hardlinks + p.specialfiles
	}
	percent := func() (int64, int64) {
		return p.metadata, metadataTotal()
	}
	progress := func() string {
		return fmt.Sprintf("Restoring filesystem timestamps and other metadata: %d / %d",
			p.metadata,
			metadataTotal(),
		)
	}
	summary := func(time.Duration) {
		p.ui.P("Restored %d filesystem timestamps and other metadata", p.metadata)
	}
	p.ui.StartPhase(progress, nil, percent, summary)
}

// announce start of file content verification phase
func (p *progressUI) startVerify() {
	progress := func() string {
		return fmt.Sprintf("Verifying files content: %s / %s %d / %d files",
			ui.FormatBytes(p.bytes.verified),
			ui.FormatBytes(p.bytes.total),
			p.files.verified,
			p.files.total,
		)
	}
	percent := func() (int64, int64) {
		return p.bytes.verified, p.bytes.total
	}
	summary := func(time.Duration) {
		// TODO speed
		p.ui.P("Verified %s in %d files",
			ui.FormatBytes(p.bytes.verified),
			p.files.verified,
		)
	}
	p.ui.StartPhase(progress, nil, percent, summary)
}

// show restore summary and statistics
func (p *progressUI) done() {
	// TODO decide if this is useful
	// individual subtotals already provide good idea of the work done
	// summary := func() []string {
	// }
	// p.ui.Set("", nil, summary)
}
