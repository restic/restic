package progress_test

import (
	"testing"
	"time"

	"github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui/progress"
)

func TestCounter(t *testing.T) {
	const N = 100

	var (
		finalSeen  = false
		increasing = true
		last       uint64
		ncalls     int
	)

	report := func(value uint64, d time.Duration, final bool) {
		finalSeen = true
		if value < last {
			increasing = false
		}
		last = value
		ncalls++
	}
	c := progress.New(10*time.Millisecond, report)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < N; i++ {
			time.Sleep(time.Millisecond)
			c.Add(1)
		}
	}()

	<-done
	c.Done()

	test.Assert(t, finalSeen, "final call did not happen")
	test.Assert(t, increasing, "values not increasing")
	test.Equals(t, uint64(N), last)

	t.Log("number of calls:", ncalls)
}

func TestCounterNil(t *testing.T) {
	// Shouldn't panic.
	var c *progress.Counter = nil
	c.Add(1)
	c.Done()
}
