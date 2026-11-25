package backup

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui"
)

func createJSONProgress() (*ui.MockTerminal, ProgressPrinter) {
	term := &ui.MockTerminal{}
	printer := NewJSONProgress(term, 3)
	return term, printer
}

func TestJSONError(t *testing.T) {
	term, printer := createJSONProgress()
	test.Equals(t, printer.Error("/path", errors.New("error \"message\"")), nil)
	test.Equals(t, []string{"{\"message_type\":\"error\",\"error\":{\"message\":\"error \\\"message\\\"\"},\"during\":\"archival\",\"item\":\"/path\"}\n"}, term.Errors)
}

func TestJSONScannerError(t *testing.T) {
	term, printer := createJSONProgress()
	test.Equals(t, printer.ScannerError("/path", errors.New("error \"message\"")), nil)
	test.Equals(t, []string{"{\"message_type\":\"error\",\"error\":{\"message\":\"error \\\"message\\\"\"},\"during\":\"scan\",\"item\":\"/path\"}\n"}, term.Errors)
}

func TestJSONUpdateIncludesRates(t *testing.T) {
	term, printer := createJSONProgress()
	printer.Update(Counter{}, Counter{Bytes: 1024 * 1024}, 0, nil, time.Now(), 0,
		2*1024*1024.0, 1*1024*1024.0)

	if len(term.Output) != 1 {
		t.Fatalf("expected exactly one JSON line, got %d", len(term.Output))
	}
	var msg map[string]any
	if err := json.Unmarshal([]byte(strings.TrimRight(term.Output[0], "\n")), &msg); err != nil {
		t.Fatalf("invalid JSON status: %v", err)
	}
	if got, _ := msg["current_rate"].(float64); got != 2*1024*1024 {
		t.Errorf("current_rate = %v, want %v", got, 2*1024*1024)
	}
	if got, _ := msg["overall_rate"].(float64); got != 1*1024*1024 {
		t.Errorf("overall_rate = %v, want %v", got, 1*1024*1024)
	}
}
