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
	printer.Update(State{3, 11, 0, 29, 47, 0}, 5*time.Second)
	test.Equals(t, []string{"[0:05] 61.70%  3 files/dirs 29 B, total 11 files/dirs 47 B"}, term.output)
}

func TestPrintUpdateWithSkipped(t *testing.T) {
	term := &mockTerm{}
	printer := NewTextProgress(term)
	printer.Update(State{3, 11, 2, 29, 47, 59}, 5*time.Second)
	test.Equals(t, []string{"[0:05] 61.70%  3 files/dirs 29 B, total 11 files/dirs 47 B, skipped 2 files/dirs 59 B"}, term.output)
}

func TestPrintSummaryOnSuccess(t *testing.T) {
	term := &mockTerm{}
	printer := NewTextProgress(term)
	printer.Finish(State{11, 11, 0, 47, 47, 0}, 5*time.Second)
	test.Equals(t, []string{"Summary: Restored 11 files/dirs (47 B) in 0:05"}, term.output)
}

func TestPrintSummaryOnErrors(t *testing.T) {
	term := &mockTerm{}
	printer := NewTextProgress(term)
	printer.Finish(State{3, 11, 0, 29, 47, 0}, 5*time.Second)
	test.Equals(t, []string{"Summary: Restored 3 / 11 files/dirs (29 B / 47 B) in 0:05"}, term.output)
}

func TestPrintSummaryOnSuccessWithSkipped(t *testing.T) {
	term := &mockTerm{}
	printer := NewTextProgress(term)
	printer.Finish(State{11, 11, 2, 47, 47, 59}, 5*time.Second)
	test.Equals(t, []string{"Summary: Restored 11 files/dirs (47 B) in 0:05, skipped 2 files/dirs 59 B"}, term.output)
}
