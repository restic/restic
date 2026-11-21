package backup

import (
	"strings"
	"testing"
	"time"

	"github.com/restic/restic/internal/archiver"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui"
)

func createTextProgress() (*ui.MockTerminal, ProgressPrinter) {
	term := &ui.MockTerminal{}
	printer := NewTextProgress(term, 3)
	return term, printer
}

func TestError(t *testing.T) {
	term, printer := createTextProgress()
	test.Equals(t, printer.Error("/path", errors.New("error \"message\"")), nil)
	test.Equals(t, []string{"error: error \"message\"\n"}, term.Errors)
}

func TestScannerError(t *testing.T) {
	term, printer := createTextProgress()
	test.Equals(t, printer.ScannerError("/path", errors.New("error \"message\"")), nil)
	test.Equals(t, []string{"scan: error \"message\"\n"}, term.Errors)
}

func TestFormatMiBPerSecond(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0 MiB/s"},
		{1024, "1.00 KiB/s"},
		{1.5 * 1024 * 1024, "1.50 MiB/s"},
		{2.75 * 1024 * 1024 * 1024, "2.75 GiB/s"},
	}

	for _, c := range cases {
		if got := formatMiBPerSecond(c.in); got != c.want {
			t.Errorf("formatMiBPerSecond(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestUpdateRatesSuffix(t *testing.T) {
	term, printer := createTextProgress()
	now := time.Now()

	// With processed bytes the rate suffix should be appended.
	printer.Update(Counter{Bytes: 1024 * 1024}, Counter{Bytes: 1024 * 1024}, 0, nil, now, 0,
		2*1024*1024.0, 1*1024*1024.0)
	if !strings.HasSuffix(term.Output[0], ", current 2.00 MiB/s, avg 1.00 MiB/s") {
		t.Errorf("status line missing rate suffix: %q", term.Output[0])
	}

	// Without processed bytes the rate suffix must not appear.
	printer.Update(Counter{}, Counter{}, 0, nil, now, 0, 0, 0)
	for _, line := range term.Output {
		if strings.Contains(line, "current ") || strings.Contains(line, "avg ") {
			t.Errorf("status line unexpectedly contains rate suffix: %q", line)
		}
	}
}

func TestFinishSummaryRate(t *testing.T) {
	term, printer := createTextProgress()
	// 2 MiB processed over 1 second => 2 MiB/s average.
	start := time.Now()
	end := start.Add(time.Second)
	summary := &archiver.Summary{
		BackupStart:    start,
		BackupEnd:      end,
		ProcessedBytes: 2 * 1024 * 1024,
	}
	printer.Finish(restic.ID{}, summary, true)

	var processedLine string
	for _, line := range term.Output {
		if strings.HasPrefix(line, "processed ") {
			processedLine = line
			break
		}
	}
	if processedLine == "" {
		t.Fatalf("no processed line in output: %q", term.Output)
	}
	if !strings.HasSuffix(strings.TrimRight(processedLine, "\n"), ", avg 2.00 MiB/s") {
		t.Errorf("processed line missing avg rate suffix: %q", processedLine)
	}

	// Zero processed bytes => no rate suffix.
	term, printer = createTextProgress()
	summary.ProcessedBytes = 0
	printer.Finish(restic.ID{}, summary, true)
	for _, line := range term.Output {
		if strings.Contains(line, "processed ") && strings.Contains(line, "avg ") {
			t.Errorf("processed line unexpectedly contains rate suffix with zero bytes: %q", line)
		}
	}
}
