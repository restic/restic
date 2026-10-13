package dump

import (
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

func TestJSONPrintUpdate(t *testing.T) {
	term, printer := createJSONProgress()
	printer.Update(State{10, 5, 15, 1000}, 5*time.Second)
	test.Equals(t, []string{"{\"message_type\":\"status\",\"seconds_elapsed\":5,\"files_processed\":10,\"dirs_processed\":5,\"total_items\":15,\"bytes_processed\":1000}\n"}, term.Output)
}

func TestJSONPrintSummary(t *testing.T) {
	term, printer := createJSONProgress()
	printer.Finish(State{20, 10, 30, 2000}, 10*time.Second)
	test.Equals(t, []string{"{\"message_type\":\"summary\",\"seconds_elapsed\":10,\"files_processed\":20,\"dirs_processed\":10,\"total_items\":30,\"bytes_processed\":2000}\n"}, term.Output)
}

func TestJSONPrintCompleteItem(t *testing.T) {
	for _, data := range []struct {
		nodeType string
		size     uint64
		expected string
	}{
		{"dir", 0, "{\"message_type\":\"verbose_status\",\"action\":\"dumped\",\"node_type\":\"dir\",\"item\":\"test\",\"size\":0}\n"},
		{"file", 123, "{\"message_type\":\"verbose_status\",\"action\":\"dumped\",\"node_type\":\"file\",\"item\":\"test\",\"size\":123}\n"},
		{"symlink", 0, "{\"message_type\":\"verbose_status\",\"action\":\"dumped\",\"node_type\":\"symlink\",\"item\":\"test\",\"size\":0}\n"},
	} {
		term, printer := createJSONProgress()
		printer.CompleteItem("test", data.size, data.nodeType)
		test.Equals(t, []string{data.expected}, term.Output)
	}
}

func TestJSONError(t *testing.T) {
	term, printer := createJSONProgress()
	test.Equals(t, printer.Error("/path", errors.New("error \"message\"")), nil)
	test.Equals(t, []string{"{\"message_type\":\"error\",\"error\":{\"message\":\"error \\\"message\\\"\"},\"during\":\"dump\",\"item\":\"/path\"}\n"}, term.Errors)
}
