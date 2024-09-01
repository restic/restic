package restore

import (
	"sync"
	"time"

	"github.com/restic/restic/internal/ui/progress"
)

type State struct {
	FilesFinished   uint64
	FilesTotal      uint64
	FilesSkipped    uint64
	AllBytesWritten uint64
	AllBytesTotal   uint64
	AllBytesSkipped uint64
}

type Progress struct {
	updater progress.Updater
	m       sync.Mutex

	progressInfoMap map[string]progressInfoEntry
	s               State
	started         time.Time

	printer ProgressPrinter
}

type progressInfoEntry struct {
	bytesWritten uint64
	bytesTotal   uint64
}

type ProgressPrinter interface {
	Update(progress State, duration time.Duration)
	Error(item string, err error) error
	CompleteItem(action ItemAction, item string, size uint64)
	Finish(progress State, duration time.Duration)
}

type ItemAction string

// Constants for the different CompleteItem actions.
const (
	ActionDirRestored   ItemAction = "dir restored"
	ActionFileRestored  ItemAction = "file restored"
	ActionFileUpdated   ItemAction = "file updated"
	ActionFileUnchanged ItemAction = "file unchanged"
	ActionOtherRestored ItemAction = "other restored"
	ActionDeleted       ItemAction = "deleted"
)

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
		p.printer.Update(p.s, runtime)
	} else {
		p.printer.Finish(p.s, runtime)
	}
}

// AddFile starts tracking a new file with the given size
func (p *Progress) AddFile(size uint64) {
	if p == nil {
		return
	}

	p.m.Lock()
	defer p.m.Unlock()

	p.s.FilesTotal++
	p.s.AllBytesTotal += size
}

// AddProgress accumulates the number of bytes written for a file
func (p *Progress) AddProgress(name string, action ItemAction, bytesWrittenPortion uint64, bytesTotal uint64) {
	if p == nil {
		return
	}

	p.m.Lock()
	defer p.m.Unlock()

	entry, exists := p.progressInfoMap[name]
	if !exists {
		entry.bytesTotal = bytesTotal
	}
	entry.bytesWritten += bytesWrittenPortion
	p.progressInfoMap[name] = entry

	p.s.AllBytesWritten += bytesWrittenPortion
	if entry.bytesWritten == entry.bytesTotal {
		delete(p.progressInfoMap, name)
		p.s.FilesFinished++

		p.printer.CompleteItem(action, name, bytesTotal)
	}
}

func (p *Progress) AddSkippedFile(name string, size uint64) {
	if p == nil {
		return
	}

	p.m.Lock()
	defer p.m.Unlock()

	p.s.FilesSkipped++
	p.s.AllBytesSkipped += size

	p.printer.CompleteItem(ActionFileUnchanged, name, size)
}

func (p *Progress) ReportDeletedFile(name string) {
	if p == nil {
		return
	}

	p.m.Lock()
	defer p.m.Unlock()

	p.printer.CompleteItem(ActionDeleted, name, 0)
}

func (p *Progress) Error(item string, err error) error {
	if p == nil {
		return nil
	}

	p.m.Lock()
	defer p.m.Unlock()

	return p.printer.Error(item, err)
}

func (p *Progress) Finish() {
	p.updater.Done()
}
