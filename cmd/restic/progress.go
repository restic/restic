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
// for non-interactive terminals or when run using the --quiet flag
func calculateProgressInterval(show bool) time.Duration {
	interval := time.Second / 60
	fps, err := strconv.ParseFloat(os.Getenv("RESTIC_PROGRESS_FPS"), 64)
	if err == nil && fps > 0 {
		if fps > 60 {
			fps = 60
		}
		interval = time.Duration(float64(time.Second) / fps)
	} else if !stdoutCanUpdateStatus() || !show {
		interval = 0
	}
	return interval
}

// newProgressMax returns a progress.Counter that prints to stdout.
func newProgressMax(show bool, max uint64, description string) *progress.Counter {
	if !show {
		return nil
	}
	interval := calculateProgressInterval(show)

	return progress.New(interval, max, func(v uint64, max uint64, d time.Duration, final bool) {
		var status string
		if max == 0 {
			status = fmt.Sprintf("[%s]          %d %s", formatDuration(d), v, description)
		} else {
			status = fmt.Sprintf("[%s] %s  %d / %d %s",
				formatDuration(d), formatPercent(v, max), v, max, description)
		}

		if w := stdoutTerminalWidth(); w > 0 {
			status = shortenStatus(w, status)
		}

		PrintProgress("%s", status)
		if final {
			fmt.Print("\n")
		}
	})
}
