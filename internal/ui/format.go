package ui

import (
	"fmt"
	"time"
)

func FormatBytes(c uint64) string {
	b := float64(c)
	switch {
	case c >= 1<<40:
		return fmt.Sprintf("%.3f TiB", b/(1<<40))
	case c >= 1<<30:
		return fmt.Sprintf("%.3f GiB", b/(1<<30))
	case c >= 1<<20:
		return fmt.Sprintf("%.3f MiB", b/(1<<20))
	case c >= 1<<10:
		return fmt.Sprintf("%.3f KiB", b/(1<<10))
	default:
		return fmt.Sprintf("%d B", c)
	}
}

// FormatPercent formats numerator/denominator as a percentage.
func FormatPercent(numerator uint64, denominator uint64) string {
	if denominator == 0 {
		return ""
	}

	percent := 100.0 * float64(numerator) / float64(denominator)
	if percent > 100 {
		percent = 100
	}

	return fmt.Sprintf("%3.2f%%", percent)
}

// FormatDuration formats d as FormatSeconds would.
func FormatDuration(d time.Duration) string {
	sec := uint64(d / time.Second)
	return FormatSeconds(sec)
}

// FormatSeconds formats sec as MM:SS, or HH:MM:SS if sec seconds
// is at least an hour.
func FormatSeconds(sec uint64) string {
	hours := sec / 3600
	sec -= hours * 3600
	min := sec / 60
	sec -= min * 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, min, sec)
	}
	return fmt.Sprintf("%d:%02d", min, sec)
}
