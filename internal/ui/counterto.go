package ui

import (
	"time"
)

type Stopwatch struct {
	start time.Time
}

func StartStopwatch() Stopwatch {
	return Stopwatch{start: time.Now()}
}

func (s Stopwatch) Elapsed() time.Duration {
	return time.Since(s.start)
}

func (s Stopwatch) FormatDuration() string {
	return formatDuration(s.Elapsed())
}

type CounterTo struct {
	Current, Total int64
}

func StartCountTo(total int64) CounterTo {
	return CounterTo{Total: total}
}
