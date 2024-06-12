package restore

import (
	"testing"
	"time"

	"github.com/restic/restic/internal/test"
)

type printerTraceEntry struct {
	progress State

	duration   time.Duration
	isFinished bool
}

type printerTrace []printerTraceEntry

type mockPrinter struct {
	trace printerTrace
}

const mockFinishDuration = 42 * time.Second

func (p *mockPrinter) Update(progress State, duration time.Duration) {
	p.trace = append(p.trace, printerTraceEntry{progress, duration, false})
}
func (p *mockPrinter) Finish(progress State, _ time.Duration) {
	p.trace = append(p.trace, printerTraceEntry{progress, mockFinishDuration, true})
}

func testProgress(fn func(progress *Progress) bool) printerTrace {
	printer := &mockPrinter{}
	progress := NewProgress(printer, 0)
	final := fn(progress)
	progress.update(0, final)
	trace := append(printerTrace{}, printer.trace...)
	// cleanup to avoid goroutine leak, but copy trace first
	progress.Finish()
	return trace
}

func TestNew(t *testing.T) {
	result := testProgress(func(progress *Progress) bool {
		return false
	})
	test.Equals(t, printerTrace{
		printerTraceEntry{State{0, 0, 0, 0, 0, 0}, 0, false},
	}, result)
}

func TestAddFile(t *testing.T) {
	fileSize := uint64(100)

	result := testProgress(func(progress *Progress) bool {
		progress.AddFile(fileSize)
		return false
	})
	test.Equals(t, printerTrace{
		printerTraceEntry{State{0, 1, 0, 0, fileSize, 0}, 0, false},
	}, result)
}

func TestFirstProgressOnAFile(t *testing.T) {
	expectedBytesWritten := uint64(5)
	expectedBytesTotal := uint64(100)

	result := testProgress(func(progress *Progress) bool {
		progress.AddFile(expectedBytesTotal)
		progress.AddProgress("test", expectedBytesWritten, expectedBytesTotal)
		return false
	})
	test.Equals(t, printerTrace{
		printerTraceEntry{State{0, 1, 0, expectedBytesWritten, expectedBytesTotal, 0}, 0, false},
	}, result)
}

func TestLastProgressOnAFile(t *testing.T) {
	fileSize := uint64(100)

	result := testProgress(func(progress *Progress) bool {
		progress.AddFile(fileSize)
		progress.AddProgress("test", 30, fileSize)
		progress.AddProgress("test", 35, fileSize)
		progress.AddProgress("test", 35, fileSize)
		return false
	})
	test.Equals(t, printerTrace{
		printerTraceEntry{State{1, 1, 0, fileSize, fileSize, 0}, 0, false},
	}, result)
}

func TestLastProgressOnLastFile(t *testing.T) {
	fileSize := uint64(100)

	result := testProgress(func(progress *Progress) bool {
		progress.AddFile(fileSize)
		progress.AddFile(50)
		progress.AddProgress("test1", 50, 50)
		progress.AddProgress("test2", 50, fileSize)
		progress.AddProgress("test2", 50, fileSize)
		return false
	})
	test.Equals(t, printerTrace{
		printerTraceEntry{State{2, 2, 0, 50 + fileSize, 50 + fileSize, 0}, 0, false},
	}, result)
}

func TestSummaryOnSuccess(t *testing.T) {
	fileSize := uint64(100)

	result := testProgress(func(progress *Progress) bool {
		progress.AddFile(fileSize)
		progress.AddFile(50)
		progress.AddProgress("test1", 50, 50)
		progress.AddProgress("test2", fileSize, fileSize)
		return true
	})
	test.Equals(t, printerTrace{
		printerTraceEntry{State{2, 2, 0, 50 + fileSize, 50 + fileSize, 0}, mockFinishDuration, true},
	}, result)
}

func TestSummaryOnErrors(t *testing.T) {
	fileSize := uint64(100)

	result := testProgress(func(progress *Progress) bool {
		progress.AddFile(fileSize)
		progress.AddFile(50)
		progress.AddProgress("test1", 50, 50)
		progress.AddProgress("test2", fileSize/2, fileSize)
		return true
	})
	test.Equals(t, printerTrace{
		printerTraceEntry{State{1, 2, 0, 50 + fileSize/2, 50 + fileSize, 0}, mockFinishDuration, true},
	}, result)
}

func TestSkipFile(t *testing.T) {
	fileSize := uint64(100)

	result := testProgress(func(progress *Progress) bool {
		progress.AddSkippedFile(fileSize)
		return true
	})
	test.Equals(t, printerTrace{
		printerTraceEntry{State{0, 0, 1, 0, 0, fileSize}, mockFinishDuration, true},
	}, result)
}
