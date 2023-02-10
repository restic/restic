package progress

import (
	"sync"
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

	valueMutex sync.Mutex
	value      uint64
	max        uint64
}

// NewCounter starts a new Counter.
func NewCounter(interval time.Duration, total uint64, report Func) *Counter {
	c := &Counter{
		max: total,
	}
	c.Updater = *NewUpdater(interval, func(runtime time.Duration, final bool) {
		v, max := c.Get()
		report(v, max, runtime, final)
	})
	return c
}

// Add v to the Counter. This method is concurrency-safe.
func (c *Counter) Add(v uint64) {
	if c == nil {
		return
	}

	c.valueMutex.Lock()
	c.value += v
	c.valueMutex.Unlock()
}

// SetMax sets the maximum expected counter value. This method is concurrency-safe.
func (c *Counter) SetMax(max uint64) {
	if c == nil {
		return
	}
	c.valueMutex.Lock()
	c.max = max
	c.valueMutex.Unlock()
}

// Get returns the current value and the maximum of c.
// This method is concurrency-safe.
func (c *Counter) Get() (v, max uint64) {
	c.valueMutex.Lock()
	v, max = c.value, c.max
	c.valueMutex.Unlock()

	return v, max
}

func (c *Counter) Done() {
	if c != nil {
		c.Updater.Done()
	}
}
