package restore

import (
	"testing"
	"time"

	"github.com/restic/restic/internal/test"
)

type mockTerm struct {
	output []string
}

func (m *mockTerm) Print(line string) {
	m.output = append(m.output, line)
}

func (m *mockTerm) SetStatus(lines []string) {
	m.output = append([]string{}, lines...)
}

func TestPrintUpdate(t *testing.T) {
	term := &mockTerm{}
	printer := NewTextProgress(term)
	printer.Update(3, 11, 29, 47, 5*time.Second)
	test.Equals(t, []string{"[0:05] 61.70%  3 files 29 B, total 11 files 47 B"}, term.output)
}

func TestPrintSummaryOnSuccess(t *testing.T) {
	term := &mockTerm{}
	printer := NewTextProgress(term)
	printer.Finish(11, 11, 47, 47, 5*time.Second)
	test.Equals(t, []string{"Summary: Restored 11 Files (47 B) in 0:05"}, term.output)
}

func TestPrintSummaryOnErrors(t *testing.T) {
	term := &mockTerm{}
	printer := NewTextProgress(term)
	printer.Finish(3, 11, 29, 47, 5*time.Second)
	test.Equals(t, []string{"Summary: Restored 3 / 11 Files (29 B / 47 B) in 0:05"}, term.output)
}
