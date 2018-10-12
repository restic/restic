package ui

import (
	"fmt"
	"time"
)

// CounterTo tracks progress of a single metric of a long running operation and provides
// info about current status and ETA of completion.
type CounterTo struct {
	start           time.Time
	target, current uint64 // XXX unsigned integers wrap at zero, it's bad
}

func StartCountTo(start time.Time, target uint64) CounterTo {
	return CounterTo{start: start, target: target}
}

func (c *CounterTo) Add(count uint) {
	c.current += uint64(count)
}

func (c *CounterTo) Percent() float64 {
	percent := 100.0 * float64(c.current) / float64(c.target)

	if percent > 100 {
		percent = 100
	}

	return percent
}

func (c *CounterTo) FormatPercent() string {
	return fmt.Sprintf("%3.2f%%", c.Percent())
}

func (c *CounterTo) Value() uint64 {
	return c.current
}

func (c *CounterTo) Target() uint64 {
	return c.target
}

func (c *CounterTo) ETA(now time.Time) time.Duration {
	if c.current >= c.target {
		return etaDONE
	}

	elapsed := now.Sub(c.start)
	current := c.current
	target := c.target

	if elapsed <= 0 || current <= 0 {
		return etaNA
	}

	// can't calculate in nanoseconds because int64 can overflow
	// will calculate in float64 seconds, then convert back to nanoseconds
	etaSec := elapsed.Seconds() * (float64(target) - float64(current)) / float64(current)
	eta := time.Duration(etaSec * float64(time.Second.Nanoseconds()))

	// fmt.Printf("elapsed=%d current=%d target=%d etaSec=%f eta=%d\n", elapsed, current, target, etaSec, eta)

	return eta
}

func (c *CounterTo) FormatETA(now time.Time) string {
	return FormatSeconds(uint64(c.ETA(now) / time.Second))
}
