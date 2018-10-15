package restorer

import (
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
	totalDirs ui.Counter

	// files are selected first, then restored, then optionally verified
	totalFiles                   ui.Counter
	totalBytes                   ui.Counter
	restoredFiles, verifiedFiles ui.CounterTo
	restoredBytes, verifiedBytes ui.CounterTo

	// TODO track downloaded bytes

	// only count hardlinks and special files
	totalHardlinks, totalSpecialFiles ui.Counter

	restoredMetadata ui.CounterTo
}

func newProgressUI(ui ui.ProgressUI) *progressUI {
	return &progressUI{ui: ui}
}

// call when a directory is created
func (p *progressUI) addDir() {
	p.ui.Update(func() { (&p.totalDirs).Add(1) })
}

// call when a file is selected
func (p *progressUI) addFile(size uint64) {
	p.ui.Update(func() {
		(&p.totalFiles).Add(1)
		(&p.totalBytes).Add(int64(size))
	})
}

// call when a hardlink is selected
func (p *progressUI) addHardlink() {
	p.ui.Update(func() { p.totalHardlinks.Add(1) })
}

// call when a special file is selected
func (p *progressUI) addSpecialFile() {
	p.ui.Update(func() { (&p.totalSpecialFiles).Add(1) })
}

// call when a file blob is written to a file
func (p *progressUI) completeBlob(size uint) {
	p.ui.Update(func() { (&p.restoredBytes).Add(int64(size)) })
}

// call when a file blob is verified
func (p *progressUI) completeVerifyBlob(size uint) {
	p.ui.Update(func() { p.verifiedBytes.Add(int64(size)) })
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
	setup := func() {}
	metrics := map[string]interface{}{
		"dirs":  &p.totalDirs,
		"files": &p.totalFiles,
		"bytes": &p.totalBytes,
	}
	progress := "{{.dirs.Value}} directories, {{.bytes.FormatBytes}} in {{.files.Value}} files"
	subtotal := "Created {{.dirs.Value}} directories, listed {{.bytes.FormatBytes}} in {{.files.Value}} files"

	p.ui.Set("Creating directories and listing files...", setup, metrics, progress, subtotal)
}

// announce start of file content download phase
func (p *progressUI) startFileContent() {
	setup := func() {
		p.restoredFiles = ui.StartCountTo(p.totalFiles.Value())
		p.restoredBytes = ui.StartCountTo(p.totalBytes.Value())
	}
	metrics := map[string]interface{}{
		"files":   &p.restoredFiles,
		"bytes":   &p.restoredBytes,
		"percent": &p.restoredBytes,
	}
	progress := "{{.bytes.FormatBytes}} / {{.bytes.Target.FormatBytes}} {{.files.Value}} / {{.files.Target.Value}} files"
	subtotal := "Restored {{.files.FormatBytes}} in {{.files.Value}} files"
	p.ui.Set("Restoring files content...", setup, metrics, progress, subtotal)
}

// announce filesystem metadata restoration phase
func (p *progressUI) startMetadata() {
	setup := func() {
		p.restoredMetadata = ui.StartCountTo(p.totalDirs.Value() + p.totalFiles.Value() + p.totalHardlinks.Value() + p.totalSpecialFiles.Value())
	}
	metrics := map[string]interface{}{
		"metadata": &p.restoredMetadata,
		"percent":  &p.restoredMetadata,
	}
	progress := "{{.metadata.Value}} / {{.metadata.Target.Value}}"
	subtotal := "Restored {{.metadata.Value}} filesystem timestamps and other metadata"
	p.ui.Set("Restoring filesystem timestamps and other metadata...", setup, metrics, progress, subtotal)
}

// announce start of file content verification phase
func (p *progressUI) startVerify() {
	setup := func() {
		p.verifiedFiles = ui.StartCountTo(p.totalFiles.Value())
		p.verifiedBytes = ui.StartCountTo(p.totalBytes.Value())
	}
	metrics := map[string]interface{}{
		"files":   &p.verifiedFiles,
		"bytes":   &p.verifiedBytes,
		"percent": &p.verifiedBytes,
	}
	progress := ""
	subtotal := "Verified {{.bytes.FormatBytes}} in {{.files.Value}} files"
	p.ui.Set("Verifying files content...", setup, metrics, progress, subtotal)
}

// show restore summary and statistics
func (p *progressUI) done() {
	// TODO decide if this is useful
	// individual subtotals already provide good idea of the work done
	// summary := func() []string {
	// }
	// p.ui.Set("", nil, summary)
}
