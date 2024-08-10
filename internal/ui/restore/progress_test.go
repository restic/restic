package restore

import (
	"testing"
	"time"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/test"
)

type printerTraceEntry struct {
	progress State

	duration   time.Duration
	isFinished bool
}

type printerTrace []printerTraceEntry

type itemTraceEntry struct {
	action ItemAction
	item   string
	size   uint64
}

type itemTrace []itemTraceEntry

type errorTraceEntry struct {
	item string
	err  error
}

type errorTrace []errorTraceEntry

type mockPrinter struct {
	trace  printerTrace
	items  itemTrace
	errors errorTrace
}

const mockFinishDuration = 42 * time.Second

func (p *mockPrinter) Update(progress State, duration time.Duration) {
	p.trace = append(p.trace, printerTraceEntry{progress, duration, false})
}
func (p *mockPrinter) Error(item string, err error) error {
	p.errors = append(p.errors, errorTraceEntry{item, err})
	return nil
}
func (p *mockPrinter) CompleteItem(action ItemAction, item string, size uint64) {
	p.items = append(p.items, itemTraceEntry{action, item, size})
}
func (p *mockPrinter) Finish(progress State, _ time.Duration) {
	p.trace = append(p.trace, printerTraceEntry{progress, mockFinishDuration, true})
}

func testProgress(fn func(progress *Progress) bool) (printerTrace, itemTrace, errorTrace) {
	printer := &mockPrinter{}
	progress := NewProgress(printer, 0)
	final := fn(progress)
	progress.update(0, final)
	trace := append(printerTrace{}, printer.trace...)
	items := append(itemTrace{}, printer.items...)
	errors := append(errorTrace{}, printer.errors...)
	// cleanup to avoid goroutine leak, but copy trace first
	progress.Finish()
	return trace, items, errors
}

func TestNew(t *testing.T) {
	result, items, _ := testProgress(func(progress *Progress) bool {
		return false
	})
	test.Equals(t, printerTrace{
		printerTraceEntry{State{0, 0, 0, 0, 0, 0}, 0, false},
	}, result)
	test.Equals(t, itemTrace{}, items)
}

func TestAddFile(t *testing.T) {
	fileSize := uint64(100)

	result, items, _ := testProgress(func(progress *Progress) bool {
		progress.AddFile(fileSize)
		return false
	})
	test.Equals(t, printerTrace{
		printerTraceEntry{State{0, 1, 0, 0, fileSize, 0}, 0, false},
	}, result)
	test.Equals(t, itemTrace{}, items)
}

func TestFirstProgressOnAFile(t *testing.T) {
	expectedBytesWritten := uint64(5)
	expectedBytesTotal := uint64(100)

	result, items, _ := testProgress(func(progress *Progress) bool {
		progress.AddFile(expectedBytesTotal)
		progress.AddProgress("test", ActionFileUpdated, expectedBytesWritten, expectedBytesTotal)
		return false
	})
	test.Equals(t, printerTrace{
		printerTraceEntry{State{0, 1, 0, expectedBytesWritten, expectedBytesTotal, 0}, 0, false},
	}, result)
	test.Equals(t, itemTrace{}, items)
}

func TestLastProgressOnAFile(t *testing.T) {
	fileSize := uint64(100)

	result, items, _ := testProgress(func(progress *Progress) bool {
		progress.AddFile(fileSize)
		progress.AddProgress("test", ActionFileUpdated, 30, fileSize)
		progress.AddProgress("test", ActionFileUpdated, 35, fileSize)
		progress.AddProgress("test", ActionFileUpdated, 35, fileSize)
		return false
	})
	test.Equals(t, printerTrace{
		printerTraceEntry{State{1, 1, 0, fileSize, fileSize, 0}, 0, false},
	}, result)
	test.Equals(t, itemTrace{
		itemTraceEntry{action: ActionFileUpdated, item: "test", size: fileSize},
	}, items)
}

func TestLastProgressOnLastFile(t *testing.T) {
	fileSize := uint64(100)

	result, items, _ := testProgress(func(progress *Progress) bool {
		progress.AddFile(fileSize)
		progress.AddFile(50)
		progress.AddProgress("test1", ActionFileUpdated, 50, 50)
		progress.AddProgress("test2", ActionFileUpdated, 50, fileSize)
		progress.AddProgress("test2", ActionFileUpdated, 50, fileSize)
		return false
	})
	test.Equals(t, printerTrace{
		printerTraceEntry{State{2, 2, 0, 50 + fileSize, 50 + fileSize, 0}, 0, false},
	}, result)
	test.Equals(t, itemTrace{
		itemTraceEntry{action: ActionFileUpdated, item: "test1", size: 50},
		itemTraceEntry{action: ActionFileUpdated, item: "test2", size: fileSize},
	}, items)
}

func TestSummaryOnSuccess(t *testing.T) {
	fileSize := uint64(100)

	result, _, _ := testProgress(func(progress *Progress) bool {
		progress.AddFile(fileSize)
		progress.AddFile(50)
		progress.AddProgress("test1", ActionFileUpdated, 50, 50)
		progress.AddProgress("test2", ActionFileUpdated, fileSize, fileSize)
		return true
	})
	test.Equals(t, printerTrace{
		printerTraceEntry{State{2, 2, 0, 50 + fileSize, 50 + fileSize, 0}, mockFinishDuration, true},
	}, result)
}

func TestSummaryOnErrors(t *testing.T) {
	fileSize := uint64(100)

	result, _, _ := testProgress(func(progress *Progress) bool {
		progress.AddFile(fileSize)
		progress.AddFile(50)
		progress.AddProgress("test1", ActionFileUpdated, 50, 50)
		progress.AddProgress("test2", ActionFileUpdated, fileSize/2, fileSize)
		return true
	})
	test.Equals(t, printerTrace{
		printerTraceEntry{State{1, 2, 0, 50 + fileSize/2, 50 + fileSize, 0}, mockFinishDuration, true},
	}, result)
}

func TestSkipFile(t *testing.T) {
	fileSize := uint64(100)

	result, items, _ := testProgress(func(progress *Progress) bool {
		progress.AddSkippedFile("test", fileSize)
		return true
	})
	test.Equals(t, printerTrace{
		printerTraceEntry{State{0, 0, 1, 0, 0, fileSize}, mockFinishDuration, true},
	}, result)
	test.Equals(t, itemTrace{
		itemTraceEntry{ActionFileUnchanged, "test", fileSize},
	}, items)
}

func TestProgressTypes(t *testing.T) {
	fileSize := uint64(100)

	_, items, _ := testProgress(func(progress *Progress) bool {
		progress.AddFile(fileSize)
		progress.AddFile(0)
		progress.AddProgress("dir", ActionDirRestored, fileSize, fileSize)
		progress.AddProgress("new", ActionFileRestored, 0, 0)
		progress.ReportDeletedFile("del")
		return true
	})
	test.Equals(t, itemTrace{
		itemTraceEntry{ActionDirRestored, "dir", fileSize},
		itemTraceEntry{ActionFileRestored, "new", 0},
		itemTraceEntry{ActionDeleted, "del", 0},
	}, items)
}

func TestProgressError(t *testing.T) {
	err1 := errors.New("err1")
	err2 := errors.New("err2")
	_, _, errors := testProgress(func(progress *Progress) bool {
		test.Equals(t, progress.Error("first", err1), nil)
		test.Equals(t, progress.Error("second", err2), nil)
		return true
	})
	test.Equals(t, errorTrace{
		errorTraceEntry{"first", err1},
		errorTraceEntry{"second", err2},
	}, errors)
}
