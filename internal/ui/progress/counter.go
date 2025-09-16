package progress

import (
	"sync/atomic"
	"time"
)

// A Func is a callback for a Counter.
//
// The final argument is true if Counter.Done has been called,
// which means that the current call will be the last.
type Func func(value uint64, total uint64, runtime time.Duration, final bool)

// A Counter tracks a running count and controls a goroutine that passes its
// value periodically to a Func.
//
// The Func is also called when SIGUSR1 (or SIGINFO, on BSD) is received.
type Counter struct {
	Updater
	value, max atomic.Uint64
}

// NewCounter starts a new Counter.
func NewCounter(interval time.Duration, total uint64, report Func) *Counter {
	c := new(Counter)
	c.max.Store(total)
	c.Updater = *NewUpdater(interval, func(runtime time.Duration, final bool) {
		v, maxV := c.Get()
		report(v, maxV, runtime, final)
	})
	return c
}

// Add v to the Counter. This method is concurrency-safe.
func (c *Counter) Add(v uint64) {
	if c != nil {
		c.value.Add(v)
	}
}

// SetMax sets the maximum expected counter value. This method is concurrency-safe.
func (c *Counter) SetMax(max uint64) {
	if c != nil {
		c.max.Store(max)
	}
}

// Get returns the current value and the maximum of c.
// This method is concurrency-safe.
func (c *Counter) Get() (v, max uint64) {
	return c.value.Load(), c.max.Load()
}

func (c *Counter) Done() {
	if c != nil {
		c.Updater.Done()
	}
}
