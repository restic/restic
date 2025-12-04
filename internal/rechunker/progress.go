package rechunker

import (
	"fmt"
	"sync"
	"time"

	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"
)

type Progress struct {
	updater progress.Updater
	m       sync.Mutex

	filesFinished  int
	filesTotal     int
	bytesProcessed uint64
	bytesTotal     uint64

	printer progress.Printer
	term    ui.Terminal
	show    bool
}

func NewProgress(term ui.Terminal, printer progress.Printer, interval time.Duration) *Progress {
	p := &Progress{
		term:    term,
		printer: printer,
	}
	p.updater = *progress.NewUpdater(interval, p.update)

	return p
}

func (p *Progress) Start(fileCount int, totalSize uint64) {
	p.m.Lock()
	defer p.m.Unlock()

	p.filesTotal = fileCount
	p.bytesTotal = totalSize
	p.show = true
}

func (p *Progress) AddFile(count int) {
	p.m.Lock()
	defer p.m.Unlock()

	p.filesFinished += count
}

func (p *Progress) AddBlob(size uint64) {
	p.m.Lock()
	defer p.m.Unlock()

	p.bytesProcessed += size
}

func (p *Progress) update(duration time.Duration, final bool) {
	p.m.Lock()
	defer p.m.Unlock()

	if p.show && !final {
		formattedDuration := ui.FormatDuration(duration)
		formattedBytesProcessed := ui.FormatBytes(p.bytesProcessed)
		formattedBytesTotal := ui.FormatBytes(p.bytesTotal)
		percent := ui.FormatPercent(p.bytesProcessed, p.bytesTotal)
		progress := []string{
			fmt.Sprintf("[%s] %v/%v distinct files processed",
				formattedDuration, p.filesFinished, p.filesTotal),
			fmt.Sprintf("%s %s/%s", percent, formattedBytesProcessed, formattedBytesTotal),
		}
		p.term.SetStatus(progress)
	} else if p.show && final {
		formattedDuration := ui.FormatDuration(duration)
		formattedBytesProcessed := ui.FormatBytes(p.bytesProcessed)
		formattedBytesTotal := ui.FormatBytes(p.bytesTotal)
		percent := ui.FormatPercent(p.bytesProcessed, p.bytesTotal)

		p.term.SetStatus(nil)
		p.printer.P("[%s] %v/%v distinct files processed\n", formattedDuration, p.filesFinished, p.filesTotal)
		p.printer.P("%s %s/%s\n", percent, formattedBytesProcessed, formattedBytesTotal)
		p.show = false
	} else {
		p.term.SetStatus(nil)
	}
}

func (p *Progress) Done() {
	if p == nil {
		return
	}

	p.updater.Done()
}
