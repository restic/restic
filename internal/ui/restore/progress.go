package restore

import (
	"sync"
	"time"

	"github.com/restic/restic/internal/ui/progress"
)

type Progress struct {
	updater progress.Updater
	m       sync.Mutex

	progressInfoMap map[string]progressInfoEntry
	filesFinished   uint64
	filesTotal      uint64
	allBytesWritten uint64
	allBytesTotal   uint64
	started         time.Time

	printer ProgressPrinter
}

type progressInfoEntry struct {
	bytesWritten uint64
	bytesTotal   uint64
}

type term interface {
	Print(line string)
	SetStatus(lines []string)
}

type ProgressPrinter interface {
	Update(filesFinished, filesTotal, allBytesWritten, allBytesTotal uint64, duration time.Duration)
	Finish(filesFinished, filesTotal, allBytesWritten, allBytesTotal uint64, duration time.Duration)
}

func NewProgress(printer ProgressPrinter, interval time.Duration) *Progress {
	p := &Progress{
		progressInfoMap: make(map[string]progressInfoEntry),
		started:         time.Now(),
		printer:         printer,
	}
	p.updater = *progress.NewUpdater(interval, p.update)
	return p
}

func (p *Progress) update(runtime time.Duration, final bool) {
	p.m.Lock()
	defer p.m.Unlock()

	if !final {
		p.printer.Update(p.filesFinished, p.filesTotal, p.allBytesWritten, p.allBytesTotal, runtime)
	} else {
		p.printer.Finish(p.filesFinished, p.filesTotal, p.allBytesWritten, p.allBytesTotal, runtime)
	}
}

// AddFile starts tracking a new file with the given size
func (p *Progress) AddFile(size uint64) {
	p.m.Lock()
	defer p.m.Unlock()

	p.filesTotal++
	p.allBytesTotal += size
}

// AddProgress accumulates the number of bytes written for a file
func (p *Progress) AddProgress(name string, bytesWrittenPortion uint64, bytesTotal uint64) {
	p.m.Lock()
	defer p.m.Unlock()

	entry, exists := p.progressInfoMap[name]
	if !exists {
		entry.bytesTotal = bytesTotal
	}
	entry.bytesWritten += bytesWrittenPortion
	p.progressInfoMap[name] = entry

	p.allBytesWritten += bytesWrittenPortion
	if entry.bytesWritten == entry.bytesTotal {
		delete(p.progressInfoMap, name)
		p.filesFinished++
	}
}

func (p *Progress) Finish() {
	p.updater.Done()
}
