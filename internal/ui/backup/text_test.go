package backup

import (
	"testing"
	"time"

	"github.com/restic/restic/internal/errors"
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

func TestTextUpdateETA(t *testing.T) {
	for _, tc := range []struct {
		secs uint64
		want string
	}{
		{1, "0h0m1s"},
		{59, "0h0m59s"},
		{60, "0h1m0s"},
		{3599, "0h59m59s"},
		{3600, "1h0m0s"},
		{80144, "22h15m44s"},
		{90061, "25h1m1s"},
		{^uint64(0), "5124095576030431h0m15s"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			term, printer := createTextProgress()
			printer.Update(Counter{Files: 2, Bytes: 100}, Counter{Files: 1, Bytes: 50}, 0, nil, time.Now(), tc.secs)
			test.Equals(t, []string{"[0:00] 50.00%  1 files 50 B, total 2 files 100 B, 0 errors ETA " + tc.want}, term.Output)
		})
	}
}

func TestTextUpdateWithoutETA(t *testing.T) {
	for _, tc := range []struct {
		name      string
		total     Counter
		processed Counter
		secs      uint64
		want      string
	}{
		{"no estimate", Counter{Files: 2, Bytes: 100}, Counter{Files: 1, Bytes: 50}, 0,
			"[0:00] 1 files 50 B, total 2 files 100 B, 0 errors"},
		{"complete", Counter{Files: 2, Bytes: 100}, Counter{Files: 2, Bytes: 100}, 60,
			"[0:00] 2 files 100 B, total 2 files 100 B, 0 errors"},
		{"no totals", Counter{}, Counter{Files: 1, Bytes: 50}, 60,
			"[0:00] 1 files, 50 B, 0 errors"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			term, printer := createTextProgress()
			printer.Update(tc.total, tc.processed, 0, nil, time.Now(), tc.secs)
			test.Equals(t, []string{tc.want}, term.Output)
		})
	}
}
