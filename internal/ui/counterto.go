package ui

import (
	"time"
)

type Counter struct {
	value int64
}

func (c *Counter) Add(count int64) {
	c.value += count
}

func (c *Counter) Set(value int64) {
	c.value = value
}

func (c *Counter) Value() int64 {
	return c.value
}

func (c *Counter) FormatBytes() string {
	return FormatBytes(uint64(c.value))
}

type Stopwatch struct {
	start time.Time
}

func StartStopwatch() *Stopwatch {
	return &Stopwatch{start: time.Now()}
}

func (s *Stopwatch) Elapsed() time.Duration {
	return time.Since(s.start)
}

func (s *Stopwatch) FormatDuration() string {
	return formatDuration(s.Elapsed())
}

type CounterTo struct {
	Counter
	target Counter
}

func StartCountTo(target int64) CounterTo {
	return CounterTo{target: Counter{value: target}}
}

func (c *CounterTo) FormatPercent() string {
	return FormatPercent(uint64(c.value), uint64(c.target.value))
}

func (c *CounterTo) ETA(sw Stopwatch) time.Duration {
	if c.value >= c.target.value {
		return etaDONE
	}

	elapsed := sw.Elapsed()
	current := c.value
	target := c.target.value

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

func (c *CounterTo) FormatETA(sw Stopwatch) string {
	return FormatDuration(c.ETA(sw))
}

func (c *CounterTo) Target() *Counter {
	return &c.target
}
