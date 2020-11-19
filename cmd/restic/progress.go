package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/restic/restic/internal/ui/progress"
)

// newProgressMax returns a progress.Counter that prints to stdout.
func newProgressMax(show bool, max uint64, description string) *progress.Counter {
	if !show {
		return nil
	}

	interval := time.Second / 60
	if !stdoutIsTerminal() {
		interval = time.Second
	} else {
		fps, err := strconv.ParseInt(os.Getenv("RESTIC_PROGRESS_FPS"), 10, 64)
		if err == nil && fps >= 1 {
			if fps > 60 {
				fps = 60
			}
			interval = time.Second / time.Duration(fps)
		}
	}

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
