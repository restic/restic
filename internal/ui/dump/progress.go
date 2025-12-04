package dump

import (
	"sync"
	"time"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/ui/progress"
)

// State represents the current progress state of the dump operation
type State struct {
	FilesProcessed uint64
	DirsProcessed  uint64
	TotalItems     uint64
	BytesProcessed uint64
}

// ProgressPrinter is an interface for printing progress information
type ProgressPrinter interface {
	Update(state State, duration time.Duration)
	Error(item string, err error) error
	CompleteItem(item string, size uint64, nodeType string)
	Finish(state State, duration time.Duration)

	progress.Printer
}

// Progress tracks and reports the progress of a dump operation
type Progress struct {
	updater progress.Updater
	m       sync.Mutex

	state   State
	started time.Time

	printer ProgressPrinter
}

// NewProgress creates a new Progress tracker
func NewProgress(printer ProgressPrinter, interval time.Duration) *Progress {
	p := &Progress{
		started: time.Now(),
		printer: printer,
	}
	p.updater = *progress.NewUpdater(interval, p.update)
	return p
}

func (p *Progress) update(runtime time.Duration, final bool) {
	p.m.Lock()
	defer p.m.Unlock()

	if !final {
		p.printer.Update(p.state, runtime)
	} else {
		p.printer.Finish(p.state, runtime)
	}
}

// AddProgress records progress for a node that has been processed
func (p *Progress) AddProgress(item string, size uint64, nodeType data.NodeType) {
	if p == nil {
		return
	}

	p.m.Lock()
	defer p.m.Unlock()

	// Increment the appropriate counter based on node type
	switch nodeType {
	case data.NodeTypeFile:
		p.state.FilesProcessed++
	case data.NodeTypeDir:
		p.state.DirsProcessed++
	}

	// Increment total items and bytes for all node types
	p.state.TotalItems++
	p.state.BytesProcessed += size

	// Convert nodeType to string for the printer
	nodeTypeStr := "file"
	switch nodeType {
	case data.NodeTypeDir:
		nodeTypeStr = "dir"
	case data.NodeTypeSymlink:
		nodeTypeStr = "symlink"
	}

	p.printer.CompleteItem(item, size, nodeTypeStr)
}

// Error reports an error that occurred during the dump operation
func (p *Progress) Error(item string, err error) error {
	if p == nil {
		return nil
	}

	p.m.Lock()
	defer p.m.Unlock()

	return p.printer.Error(item, err)
}

// Finish completes the progress reporting
func (p *Progress) Finish() {
	p.updater.Done()
}
