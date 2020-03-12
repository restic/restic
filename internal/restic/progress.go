package restic

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/ssh/terminal"
)

// minTickerTime limits how often the progress ticker is updated. It can be
// overridden using the RESTIC_PROGRESS_FPS (frames per second) environment
// variable.
var minTickerTime = time.Second / 60

var isTerminal bool
var forceUpdateProgress chan bool

var progressOnce sync.Once

func progressInit() {
	forceUpdateProgress = make(chan bool)
	isTerminal = terminal.IsTerminal(int(os.Stdout.Fd()))

	fps, err := strconv.ParseInt(os.Getenv("RESTIC_PROGRESS_FPS"), 10, 64)
	if err == nil && fps >= 1 {
		if fps > 60 {
			fps = 60
		}
		minTickerTime = time.Second / time.Duration(fps)
	}

	progressSignalInit()
}

// Progress reports progress on an operation.
type Progress struct {
	OnStart  func()
	OnUpdate ProgressFunc
	OnDone   ProgressFunc
	fnM      sync.Mutex

	cur        Stat
	curM       sync.Mutex
	start      time.Time
	c          *time.Ticker
	cancel     chan struct{}
	o          sync.Once
	d          time.Duration
	lastUpdate time.Time

	running bool
}

// Stat captures newly done parts of the operation.
type Stat struct {
	Files  uint64
	Dirs   uint64
	Bytes  uint64
	Trees  uint64
	Blobs  uint64
	Errors uint64
}

// ProgressFunc is used to report progress back to the user.
type ProgressFunc func(s Stat, runtime time.Duration, ticker bool)

// NewProgress returns a new progress reporter. When Start() is called, the
// function OnStart is executed once. Afterwards the function OnUpdate is
// called when new data arrives or at least every d interval. The function
// OnDone is called when Done() is called. Both functions are called
// synchronously and can use shared state.
func NewProgress() *Progress {
	progressOnce.Do(progressInit)

	var d time.Duration
	if isTerminal {
		d = time.Second
	}
	return &Progress{d: d}
}

// Start resets and runs the progress reporter.
func (p *Progress) Start() {
	if p == nil || p.running {
		return
	}

	p.cancel = make(chan struct{})
	p.running = true
	p.Reset()
	p.start = time.Now()
	p.c = nil
	if p.d != 0 {
		p.c = time.NewTicker(p.d)
	}

	if p.OnStart != nil {
		p.OnStart()
	}

	go p.reporter()
}

// Reset resets all statistic counters to zero.
func (p *Progress) Reset() {
	if p == nil {
		return
	}

	if !p.running {
		panic("resetting a non-running Progress")
	}

	p.curM.Lock()
	p.cur = Stat{}
	p.curM.Unlock()
}

// Report adds the statistics from s to the current state and tries to report
// the accumulated statistics via the feedback channel.
func (p *Progress) Report(s Stat) {
	if p == nil {
		return
	}

	if !p.running {
		panic("reporting in a non-running Progress")
	}

	p.curM.Lock()
	p.cur.Add(s)
	cur := p.cur
	needUpdate := false
	if isTerminal && time.Since(p.lastUpdate) > minTickerTime {
		p.lastUpdate = time.Now()
		needUpdate = true
	}
	p.curM.Unlock()

	if needUpdate {
		p.updateProgress(cur, false)
	}

}

func (p *Progress) updateProgress(cur Stat, ticker bool) {
	if p.OnUpdate == nil {
		return
	}

	p.fnM.Lock()
	p.OnUpdate(cur, time.Since(p.start), ticker)
	p.fnM.Unlock()
}

func (p *Progress) reporter() {
	if p == nil {
		return
	}

	updateProgress := func() {
		p.curM.Lock()
		cur := p.cur
		p.curM.Unlock()
		p.updateProgress(cur, true)
	}

	var ticker <-chan time.Time
	if p.c != nil {
		ticker = p.c.C
	}

	for {
		select {
		case <-ticker:
			updateProgress()
		case <-forceUpdateProgress:
			updateProgress()
		case <-p.cancel:
			if p.c != nil {
				p.c.Stop()
			}
			return
		}
	}
}

// Done closes the progress report.
func (p *Progress) Done() {
	if p == nil || !p.running {
		return
	}

	p.running = false
	p.o.Do(func() {
		close(p.cancel)
	})

	cur := p.cur

	if p.OnDone != nil {
		p.fnM.Lock()
		p.OnUpdate(cur, time.Since(p.start), false)
		p.OnDone(cur, time.Since(p.start), false)
		p.fnM.Unlock()
	}
}

// Add accumulates other into s.
func (s *Stat) Add(other Stat) {
	s.Bytes += other.Bytes
	s.Dirs += other.Dirs
	s.Files += other.Files
	s.Trees += other.Trees
	s.Blobs += other.Blobs
	s.Errors += other.Errors
}

func (s Stat) String() string {
	b := float64(s.Bytes)
	var str string

	switch {
	case s.Bytes > 1<<40:
		str = fmt.Sprintf("%.3f TiB", b/(1<<40))
	case s.Bytes > 1<<30:
		str = fmt.Sprintf("%.3f GiB", b/(1<<30))
	case s.Bytes > 1<<20:
		str = fmt.Sprintf("%.3f MiB", b/(1<<20))
	case s.Bytes > 1<<10:
		str = fmt.Sprintf("%.3f KiB", b/(1<<10))
	default:
		str = fmt.Sprintf("%dB", s.Bytes)
	}

	return fmt.Sprintf("Stat(%d files, %d dirs, %v trees, %v blobs, %d errors, %v)",
		s.Files, s.Dirs, s.Trees, s.Blobs, s.Errors, str)
}
