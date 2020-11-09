package progress

import (
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/restic/restic/internal/debug"
)

// A Func is a callback for a Counter.
//
// The final argument is true if Counter.Done has been called,
// which means that the current call will be the last.
type Func func(value uint64, runtime time.Duration, final bool)

// A Counter tracks a running count and controls a goroutine that passes its
// value periodically to a Func.
//
// The Func is also called when SIGUSR1 (or SIGINFO, on BSD) is received.
type Counter struct {
	report  Func
	start   time.Time
	stopped chan struct{} // Closed by run.
	stop    chan struct{} // Close to stop run.
	tick    *time.Ticker
	value   uint64
}

// New starts a new Counter.
func New(interval time.Duration, report Func) *Counter {
	signals.Once.Do(func() {
		signals.ch = make(chan os.Signal, 1)
		setupSignals()
	})

	c := &Counter{
		report:  report,
		start:   time.Now(),
		stopped: make(chan struct{}),
		stop:    make(chan struct{}),
		tick:    time.NewTicker(interval),
	}

	go c.run()
	return c
}

// Add v to the Counter. This method is concurrency-safe.
func (c *Counter) Add(v uint64) {
	if c == nil {
		return
	}
	atomic.AddUint64(&c.value, v)
}

// Done tells a Counter to stop and waits for it to report its final value.
func (c *Counter) Done() {
	if c == nil {
		return
	}
	c.tick.Stop()
	close(c.stop)
	<-c.stopped    // Wait for last progress report.
	*c = Counter{} // Prevent reuse.
}

func (c *Counter) get() uint64 { return atomic.LoadUint64(&c.value) }

func (c *Counter) run() {
	defer close(c.stopped)
	defer func() {
		// Must be a func so that time.Since isn't called at defer time.
		c.report(c.get(), time.Since(c.start), true)
	}()

	for {
		var now time.Time

		select {
		case now = <-c.tick.C:
		case sig := <-signals.ch:
			debug.Log("Signal received: %v\n", sig)
			now = time.Now()
		case <-c.stop:
			return
		}

		c.report(c.get(), now.Sub(c.start), false)
	}
}

// XXX The fact that signals is a single global variable means that only one
// Counter receives each incoming signal.
var signals struct {
	ch chan os.Signal
	sync.Once
}
