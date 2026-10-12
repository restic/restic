package dump

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

func TestTextPrintUpdate(t *testing.T) {
	term, printer := createTextProgress()
	printer.Update(State{10, 5, 15, 1000}, 5*time.Second)
	test.Equals(t, []string{"[0:05] 10 files, 5 dirs, 15 total items 1000 B"}, term.Output)
}

func TestTextPrintSummary(t *testing.T) {
	term, printer := createTextProgress()
	printer.Finish(State{20, 10, 30, 2000}, 10*time.Second)
	test.Equals(t, []string{"Summary: Dumped 20 files, 10 directories, 30 total items (1.953 KiB) in 0:10"}, term.Output)
}

func TestTextPrintCompleteItem(t *testing.T) {
	for _, data := range []struct {
		nodeType string
		size     uint64
		expected string
	}{
		{"dir", 0, "dumped dir test with size 0 B"},
		{"file", 123, "dumped file test with size 123 B"},
		{"symlink", 0, "dumped symlink test with size 0 B"},
	} {
		term, printer := createTextProgress()
		printer.CompleteItem("test", data.size, data.nodeType)
		test.Equals(t, []string{data.expected}, term.Output)
	}
}

func TestTextError(t *testing.T) {
	term, printer := createTextProgress()
	test.Equals(t, printer.Error("/path", errors.New("error \"message\"")), nil)
	test.Equals(t, []string{"ignoring error for /path: error \"message\"\n"}, term.Errors)
}
