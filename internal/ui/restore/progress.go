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
	FilesDeleted    uint64
	FilesCloned     uint64
	FilesCopied     uint64
	AllBytesWritten uint64
	AllBytesTotal   uint64
	AllBytesSkipped uint64
	AllBytesCloned  uint64
	AllBytesCopied  uint64
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
	progress.Printer
}

type ItemAction string

// Constants for the different CompleteItem actions.
const (
	ActionDirRestored   ItemAction = "dir restored"
	ActionFileRestored  ItemAction = "file restored"
	ActionFileUpdated   ItemAction = "file updated"
	ActionFileCloned    ItemAction = "file cloned"
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

// AddClonedFile records progress for a locally copied/cloned file. It is assumed
// that a file is never partially copied/cloned. The blockCloned flag describes
// whether the file was cloned using filesystem-specific block cloning facilities
// (i.e. "reflinks", leading to storage deduplication), or merely copied.
func (p *Progress) AddClonedFile(name string, size uint64, blockCloned bool) {
	if p == nil {
		return
	}

	p.m.Lock()
	defer p.m.Unlock()

	p.s.AllBytesWritten += size
	p.s.FilesFinished++

	if blockCloned {
		p.s.AllBytesCloned += size
		p.s.FilesCloned++
	} else {
		p.s.AllBytesCopied += size
		p.s.FilesCopied++
	}

	p.printer.CompleteItem(ActionFileCloned, name, size)
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

func (p *Progress) ReportDeletion(name string) {
	if p == nil {
		return
	}

	p.s.FilesDeleted++

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
