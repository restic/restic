package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/restic/restic/internal/ui/progress"
)

// calculateProgressInterval returns the interval configured via RESTIC_PROGRESS_FPS
// or if unset returns an interval for 60fps on interactive terminals and 0 (=disabled)
// for non-interactive terminals
func calculateProgressInterval() time.Duration {
	interval := time.Second / 60
	fps, err := strconv.ParseInt(os.Getenv("RESTIC_PROGRESS_FPS"), 10, 64)
	if err == nil && fps >= 1 {
		if fps > 60 {
			fps = 60
		}
		interval = time.Second / time.Duration(fps)
	} else if !stdoutIsTerminal() {
		interval = 0
	}
	return interval
}

// newProgressMax returns a progress.Counter that prints to stdout.
func newProgressMax(show bool, max uint64, description string) *progress.Counter {
	if !show {
		return nil
	}
	interval := calculateProgressInterval()

	return progress.New(interval, func(v uint64, d time.Duration, final bool) {
		status := fmt.Sprintf("[%s] %s  %d / %d %s",
			formatDuration(d),
			formatPercent(v, max),
			v, max, description)

		if w := stdoutTerminalWidth(); w > 0 {
			status = shortenStatus(w, status)
		}

		PrintProgress("%s", status)
		if final {
			fmt.Print("\n")
		}
	})
}
