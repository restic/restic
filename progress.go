package restic

import (
	"sync"
	"time"
)

type Progress struct {
	F   ProgressFunc
	D   ProgressFunc
	fnM sync.Mutex

	cur    Stat
	curM   sync.Mutex
	start  time.Time
	c      *time.Ticker
	cancel chan struct{}
	o      sync.Once
	d      time.Duration

	running bool
}

type Stat struct {
	Files uint64
	Dirs  uint64
	Other uint64
	Bytes uint64
}

type ProgressFunc func(s Stat, runtime time.Duration, ticker bool)

// NewProgress returns a new progress reporter. After Start() has been called,
// the function fn is called when new data arrives or at least every d
// interval. The function doneFn is called when Done() is called. Both
// functions F and D are called synchronously and can use shared state.
func NewProgress(d time.Duration) *Progress {
	return &Progress{d: d}
}

// Start runs resets and runs the progress reporter.
func (p *Progress) Start() {
	if p == nil {
		return
	}

	if p.running {
		panic("truing to reset a running Progress")
	}

	p.o = sync.Once{}
	p.cancel = make(chan struct{})
	p.running = true
	p.Reset()
	p.start = time.Now()
	p.c = time.NewTicker(p.d)

	go p.reporter()
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
	p.curM.Unlock()

	// update progress
	if p.F != nil {
		p.fnM.Lock()
		p.F(cur, time.Since(p.start), false)
		p.fnM.Unlock()
	}
}

func (p *Progress) reporter() {
	if p == nil {
		return
	}

	for {
		select {
		case <-p.c.C:
			p.curM.Lock()
			cur := p.cur
			p.curM.Unlock()

			if p.F != nil {
				p.fnM.Lock()
				p.F(cur, time.Since(p.start), true)
				p.fnM.Unlock()
			}
		case <-p.cancel:
			p.c.Stop()
			return
		}
	}
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

// Done closes the progress report.
func (p *Progress) Done() {
	if p == nil {
		return
	}

	if !p.running {
		panic("Done() called on non-running Progress")
	}

	if p.running {
		p.running = false
		p.o.Do(func() {
			close(p.cancel)
		})

		cur := p.cur

		if p.D != nil {
			p.fnM.Lock()
			p.D(cur, time.Since(p.start), false)
			p.fnM.Unlock()
		}
	}
}

// Current returns the current stat value.
func (p *Progress) Current() Stat {
	p.curM.Lock()
	s := p.cur
	p.curM.Unlock()

	return s
}

// Add accumulates other into s.
func (s *Stat) Add(other Stat) {
	s.Bytes += other.Bytes
	s.Dirs += other.Dirs
	s.Files += other.Files
	s.Other += other.Other
}
