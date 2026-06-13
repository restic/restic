package stats

import (
	"fmt"
	"sync"
	"time"

	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"
)

// Progress reports progress for the stats command.
type Progress struct {
	progress.Updater

	term          ui.Terminal
	m             sync.Mutex
	snapshotCount uint64
	show          bool

	processedSnapshotCount uint64
	processedFileCount     uint64
	processedBlobCount     uint64
	processedSize          uint64
}

// NewProgress returns a new stats progress reporter.
func NewProgress(term ui.Terminal, quiet, json bool, snapshotCount uint64) *Progress {
	p := newProgress(term, !json, snapshotCount)
	p.Updater = *progress.NewUpdater(
		progress.CalculateProgressInterval(!quiet, json, term.CanUpdateStatus()),
		p.printProgress,
	)
	return p
}

func newProgress(term ui.Terminal, show bool, snapshotCount uint64) *Progress {
	return &Progress{
		term:          term,
		snapshotCount: snapshotCount,
		show:          show,
	}
}

func (p *Progress) printProgress(runtime time.Duration, final bool) {
	if !p.show {
		return
	}
	p.m.Lock()

	progressBase := p.processedSnapshotCount
	if progressBase > 0 && !final {
		progressBase--
	}

	status := fmt.Sprintf("[%s] %s  %d / %d snapshots", ui.FormatDuration(runtime), ui.FormatPercent(progressBase, p.snapshotCount), p.processedSnapshotCount, p.snapshotCount)

	if p.processedFileCount > 0 {
		status += fmt.Sprintf(", %v files", p.processedFileCount)
	}

	if p.processedBlobCount > 0 {
		status += fmt.Sprintf(", %d blobs", p.processedBlobCount)
	}

	status += fmt.Sprintf(", %s", ui.FormatBytes(p.processedSize))
	p.m.Unlock()

	if final {
		p.term.SetStatus(nil)
		p.term.Print(status)
	} else {
		p.term.SetStatus([]string{status})
	}
}

func (p *Progress) Update(fileCount uint64, blobCount uint64, size uint64) {
	p.m.Lock()
	defer p.m.Unlock()

	p.processedFileCount += fileCount
	p.processedBlobCount += blobCount
	p.processedSize += size
}

func (p *Progress) ProcessSnapshot() {
	p.m.Lock()
	defer p.m.Unlock()

	p.processedSnapshotCount++
	p.processedFileCount = 0
	p.processedBlobCount = 0
	p.processedSize = 0
}
