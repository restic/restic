package progress_test

import (
	"testing"
	"time"

	"github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui/progress"
)

func TestCalculateProgressIntervalQuietIgnoresFPS(t *testing.T) {
	t.Setenv("RESTIC_PROGRESS_FPS", "5")

	interval := progress.CalculateProgressInterval(false, false, true)

	test.Equals(t, time.Duration(0), interval)
}

func TestCalculateProgressIntervalUsesFPSWhenShown(t *testing.T) {
	t.Setenv("RESTIC_PROGRESS_FPS", "5")

	interval := progress.CalculateProgressInterval(true, false, true)

	test.Equals(t, 200*time.Millisecond, interval)
}
