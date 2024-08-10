package restore

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

func TestPrintUpdate(t *testing.T) {
	term, printer := createTextProgress()
	printer.Update(State{3, 11, 0, 29, 47, 0}, 5*time.Second)
	test.Equals(t, []string{"[0:05] 61.70%  3 files/dirs 29 B, total 11 files/dirs 47 B"}, term.Output)
}

func TestPrintUpdateWithSkipped(t *testing.T) {
	term, printer := createTextProgress()
	printer.Update(State{3, 11, 2, 29, 47, 59}, 5*time.Second)
	test.Equals(t, []string{"[0:05] 61.70%  3 files/dirs 29 B, total 11 files/dirs 47 B, skipped 2 files/dirs 59 B"}, term.Output)
}

func TestPrintSummaryOnSuccess(t *testing.T) {
	term, printer := createTextProgress()
	printer.Finish(State{11, 11, 0, 47, 47, 0}, 5*time.Second)
	test.Equals(t, []string{"Summary: Restored 11 files/dirs (47 B) in 0:05"}, term.Output)
}

func TestPrintSummaryOnErrors(t *testing.T) {
	term, printer := createTextProgress()
	printer.Finish(State{3, 11, 0, 29, 47, 0}, 5*time.Second)
	test.Equals(t, []string{"Summary: Restored 3 / 11 files/dirs (29 B / 47 B) in 0:05"}, term.Output)
}

func TestPrintSummaryOnSuccessWithSkipped(t *testing.T) {
	term, printer := createTextProgress()
	printer.Finish(State{11, 11, 2, 47, 47, 59}, 5*time.Second)
	test.Equals(t, []string{"Summary: Restored 11 files/dirs (47 B) in 0:05, skipped 2 files/dirs 59 B"}, term.Output)
}

func TestPrintCompleteItem(t *testing.T) {
	for _, data := range []struct {
		action   ItemAction
		size     uint64
		expected string
	}{
		{ActionDirRestored, 0, "restored  test"},
		{ActionFileRestored, 123, "restored  test with size 123 B"},
		{ActionOtherRestored, 0, "restored  test"},
		{ActionFileUpdated, 123, "updated   test with size 123 B"},
		{ActionFileUnchanged, 123, "unchanged test with size 123 B"},
		{ActionDeleted, 0, "deleted   test"},
	} {
		term, printer := createTextProgress()
		printer.CompleteItem(data.action, "test", data.size)
		test.Equals(t, []string{data.expected}, term.Output)
	}
}

func TestError(t *testing.T) {
	term, printer := createTextProgress()
	test.Equals(t, printer.Error("/path", errors.New("error \"message\"")), nil)
	test.Equals(t, []string{"ignoring error for /path: error \"message\"\n"}, term.Errors)
}
