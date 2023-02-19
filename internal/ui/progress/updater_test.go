package progress_test

import (
	"testing"
	"time"

	"github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui/progress"
)

func TestUpdater(t *testing.T) {
	finalSeen := false
	var ncalls int

	report := func(d time.Duration, final bool) {
		if final {
			finalSeen = true
		}
		ncalls++
	}
	c := progress.NewUpdater(10*time.Millisecond, report)
	time.Sleep(100 * time.Millisecond)
	c.Done()

	test.Assert(t, finalSeen, "final call did not happen")
	test.Assert(t, ncalls > 0, "no progress was reported")
}

func TestUpdaterStopTwice(t *testing.T) {
	c := progress.NewUpdater(0, func(runtime time.Duration, final bool) {})
	c.Done()
	c.Done()
}

func TestUpdaterNoTick(t *testing.T) {
	finalSeen := false
	otherSeen := false

	report := func(d time.Duration, final bool) {
		if final {
			finalSeen = true
		} else {
			otherSeen = true
		}
	}
	c := progress.NewUpdater(0, report)
	time.Sleep(time.Millisecond)
	c.Done()

	test.Assert(t, finalSeen, "final call did not happen")
	test.Assert(t, !otherSeen, "unexpected status update")
}
