package restore

import (
	"testing"
	"time"

	"github.com/restic/restic/internal/test"
)

func createJSONProgress() (*mockTerm, ProgressPrinter) {
	term := &mockTerm{}
	printer := NewJSONProgress(term, 3)
	return term, printer
}

func TestJSONPrintUpdate(t *testing.T) {
	term, printer := createJSONProgress()
	printer.Update(State{3, 11, 0, 29, 47, 0}, 5*time.Second)
	test.Equals(t, []string{"{\"message_type\":\"status\",\"seconds_elapsed\":5,\"percent_done\":0.6170212765957447,\"total_files\":11,\"files_restored\":3,\"total_bytes\":47,\"bytes_restored\":29}\n"}, term.output)
}

func TestJSONPrintUpdateWithSkipped(t *testing.T) {
	term, printer := createJSONProgress()
	printer.Update(State{3, 11, 2, 29, 47, 59}, 5*time.Second)
	test.Equals(t, []string{"{\"message_type\":\"status\",\"seconds_elapsed\":5,\"percent_done\":0.6170212765957447,\"total_files\":11,\"files_restored\":3,\"files_skipped\":2,\"total_bytes\":47,\"bytes_restored\":29,\"bytes_skipped\":59}\n"}, term.output)
}

func TestJSONPrintSummaryOnSuccess(t *testing.T) {
	term, printer := createJSONProgress()
	printer.Finish(State{11, 11, 0, 47, 47, 0}, 5*time.Second)
	test.Equals(t, []string{"{\"message_type\":\"summary\",\"seconds_elapsed\":5,\"total_files\":11,\"files_restored\":11,\"total_bytes\":47,\"bytes_restored\":47}\n"}, term.output)
}

func TestJSONPrintSummaryOnErrors(t *testing.T) {
	term, printer := createJSONProgress()
	printer.Finish(State{3, 11, 0, 29, 47, 0}, 5*time.Second)
	test.Equals(t, []string{"{\"message_type\":\"summary\",\"seconds_elapsed\":5,\"total_files\":11,\"files_restored\":3,\"total_bytes\":47,\"bytes_restored\":29}\n"}, term.output)
}

func TestJSONPrintSummaryOnSuccessWithSkipped(t *testing.T) {
	term, printer := createJSONProgress()
	printer.Finish(State{11, 11, 2, 47, 47, 59}, 5*time.Second)
	test.Equals(t, []string{"{\"message_type\":\"summary\",\"seconds_elapsed\":5,\"total_files\":11,\"files_restored\":11,\"files_skipped\":2,\"total_bytes\":47,\"bytes_restored\":47,\"bytes_skipped\":59}\n"}, term.output)
}

func TestJSONPrintCompleteItem(t *testing.T) {
	for _, data := range []struct {
		action   ItemAction
		size     uint64
		expected string
	}{
		{ActionDirRestored, 0, "{\"message_type\":\"verbose_status\",\"action\":\"restored\",\"item\":\"test\",\"size\":0}\n"},
		{ActionFileRestored, 123, "{\"message_type\":\"verbose_status\",\"action\":\"restored\",\"item\":\"test\",\"size\":123}\n"},
		{ActionFileUpdated, 123, "{\"message_type\":\"verbose_status\",\"action\":\"updated\",\"item\":\"test\",\"size\":123}\n"},
		{ActionFileUnchanged, 123, "{\"message_type\":\"verbose_status\",\"action\":\"unchanged\",\"item\":\"test\",\"size\":123}\n"},
		{ActionDeleted, 0, "{\"message_type\":\"verbose_status\",\"action\":\"deleted\",\"item\":\"test\",\"size\":0}\n"},
	} {
		term, printer := createJSONProgress()
		printer.CompleteItem(data.action, "test", data.size)
		test.Equals(t, []string{data.expected}, term.output)
	}
}
