package progress

import (
	"time"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/ui/signals"
)

// An UpdateFunc is a callback for a (progress) Updater.
//
// The final argument is true if Updater.Done has been called,
// which means that the current call will be the last.
type UpdateFunc func(runtime time.Duration, final bool)

// An Updater controls a goroutine that periodically calls an UpdateFunc.
//
// The UpdateFunc is also called when SIGUSR1 (or SIGINFO, on BSD) is received.
type Updater struct {
	report  UpdateFunc
	start   time.Time
	stopped chan struct{} // Closed by run.
	stop    chan struct{} // Close to stop run.
	tick    *time.Ticker
}

// NewUpdater starts a new Updater.
func NewUpdater(interval time.Duration, report UpdateFunc) *Updater {
	c := &Updater{
		report:  report,
		start:   time.Now(),
		stopped: make(chan struct{}),
		stop:    make(chan struct{}),
	}

	if interval > 0 {
		c.tick = time.NewTicker(interval)
	}

	go c.run()
	return c
}

// Done tells an Updater to stop and waits for it to report its final value.
// Later calls do nothing.
func (c *Updater) Done() {
	if c == nil || c.stop == nil {
		return
	}
	if c.tick != nil {
		c.tick.Stop()
	}
	close(c.stop)
	<-c.stopped // Wait for last progress report.
	c.stop = nil
}

func (c *Updater) run() {
	defer close(c.stopped)
	defer func() {
		// Must be a func so that time.Since isn't called at defer time.
		c.report(time.Since(c.start), true)
	}()

	var tick <-chan time.Time
	if c.tick != nil {
		tick = c.tick.C
	}
	signalsCh := signals.GetProgressChannel()
	for {
		var now time.Time

		select {
		case now = <-tick:
		case sig := <-signalsCh:
			debug.Log("Signal received: %v\n", sig)
			now = time.Now()
		case <-c.stop:
			return
		}

		c.report(now.Sub(c.start), false)
	}
}
