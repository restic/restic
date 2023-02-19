package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"
	"github.com/restic/restic/internal/ui/termstatus"
)

// calculateProgressInterval returns the interval configured via RESTIC_PROGRESS_FPS
// or if unset returns an interval for 60fps on interactive terminals and 0 (=disabled)
// for non-interactive terminals or when run using the --quiet flag
func calculateProgressInterval(show bool, json bool) time.Duration {
	interval := time.Second / 60
	fps, err := strconv.ParseFloat(os.Getenv("RESTIC_PROGRESS_FPS"), 64)
	if err == nil && fps > 0 {
		if fps > 60 {
			fps = 60
		}
		interval = time.Duration(float64(time.Second) / fps)
	} else if !json && !stdoutCanUpdateStatus() || !show {
		interval = 0
	}
	return interval
}

// newProgressMax returns a progress.Counter that prints to stdout.
func newProgressMax(show bool, max uint64, description string) *progress.Counter {
	if !show {
		return nil
	}
	interval := calculateProgressInterval(show, false)
	canUpdateStatus := stdoutCanUpdateStatus()

	return progress.NewCounter(interval, max, func(v uint64, max uint64, d time.Duration, final bool) {
		var status string
		if max == 0 {
			status = fmt.Sprintf("[%s]          %d %s",
				ui.FormatDuration(d), v, description)
		} else {
			status = fmt.Sprintf("[%s] %s  %d / %d %s",
				ui.FormatDuration(d), ui.FormatPercent(v, max), v, max, description)
		}

		printProgress(status, canUpdateStatus)
		if final {
			fmt.Print("\n")
		}
	})
}

func printProgress(status string, canUpdateStatus bool) {
	w := stdoutTerminalWidth()
	if w > 0 {
		if w < 3 {
			status = termstatus.Truncate(status, w)
		} else {
			trunc := termstatus.Truncate(status, w-3)
			if len(trunc) < len(status) {
				status = trunc + "..."
			}
		}
	}

	var carriageControl, clear string

	if canUpdateStatus {
		clear = clearLine(w)
	}

	if !(strings.HasSuffix(status, "\r") || strings.HasSuffix(status, "\n")) {
		if canUpdateStatus {
			carriageControl = "\r"
		} else {
			carriageControl = "\n"
		}
	}

	_, _ = os.Stdout.Write([]byte(clear + status + carriageControl))
}
