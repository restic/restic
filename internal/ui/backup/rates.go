package backup

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// DefaultRateWindow is the default time window in seconds over which
// to calculate the current transfer rate
const DefaultRateWindow = 15

// GetRateWindow returns the configured rate window in seconds from the
// environment variable RESTIC_RATE_WINDOW or the default value
func GetRateWindow() time.Duration {
	windowStr := os.Getenv("RESTIC_RATE_WINDOW")
	if windowStr == "" {
		return DefaultRateWindow * time.Second
	}

	window, err := strconv.Atoi(windowStr)
	if err != nil || window <= 0 {
		return DefaultRateWindow * time.Second
	}

	return time.Duration(window) * time.Second
}

// FormatRate formats a rate in bytes per second in a human-readable way
func FormatRate(bytesPerSecond float64) string {
	if bytesPerSecond <= 0 {
		return "0 B/s"
	}

	units := []string{"B/s", "KiB/s", "MiB/s", "GiB/s", "TiB/s"}
	value := bytesPerSecond

	var unitIndex int
	for unitIndex = 0; value >= 1024 && unitIndex < len(units)-1; unitIndex++ {
		value /= 1024
	}

	// Format with appropriate precision based on size
	var formattedValue string
	if value >= 100 {
		formattedValue = fmt.Sprintf("%.0f", value)
	} else if value >= 10 {
		formattedValue = fmt.Sprintf("%.1f", value)
	} else {
		formattedValue = fmt.Sprintf("%.2f", value)
	}

	// Trim trailing zeros and decimal point if needed
	formattedValue = strings.TrimRight(strings.TrimRight(formattedValue, "0"), ".")

	return fmt.Sprintf("%s %s", formattedValue, units[unitIndex])
}

// GetCurrentRate returns the current transfer rate over the specified time window
func (r *rateEstimator) GetCurrentRate(now time.Time, window time.Duration) float64 {
	// If window is less than or equal to zero, use the default window
	if window <= 0 {
		window = DefaultRateWindow * time.Second
	}

	if r.buckets.Len() == 0 {
		return 0
	}

	windowStart := now.Add(-window)
	var bytesInWindow uint64
	var oldestInWindow time.Time

	found := false
	for e := r.buckets.Back(); e != nil; e = e.Prev() {
		b := e.Value.(*rateBucket)
		bucketStart := b.end.Add(-bucketWidth)

		if bucketStart.Before(windowStart) {
			// This bucket starts before our window
			if !found {
				// If we didn't find any buckets completely in our window,
				// use a partial count from this bucket
				ratio := now.Sub(windowStart).Seconds() / bucketWidth.Seconds()
				if ratio > 1 {
					ratio = 1
				}
				bytesInWindow = uint64(float64(b.totalBytes) * ratio)
				oldestInWindow = windowStart
			}
			break
		}

		bytesInWindow += b.totalBytes
		if !found {
			oldestInWindow = bucketStart
			found = true
		}
	}

	if !found {
		return 0
	}

	elapsed := now.Sub(oldestInWindow).Seconds()
	if elapsed <= 0 {
		return 0
	}

	return float64(bytesInWindow) / elapsed
}

// CalculateOverallRate calculates the overall transfer rate since the start time
func CalculateOverallRate(processedBytes uint64, start time.Time) float64 {
	elapsed := time.Since(start).Seconds()
	if elapsed <= 0 {
		return 0
	}
	return float64(processedBytes) / elapsed
}
