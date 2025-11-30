package dump

import (
	"testing"
	"time"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui/progress"
)

type printerTraceEntry struct {
	progress   State
	duration   time.Duration
	isFinished bool
}

type printerTrace []printerTraceEntry

type itemTraceEntry struct {
	item     string
	size     uint64
	nodeType string
}

type itemTrace []itemTraceEntry

type errorTraceEntry struct {
	item string
	err  error
}

type errorTrace []errorTraceEntry

type mockPrinter struct {
	progress.NoopPrinter
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

func (p *mockPrinter) CompleteItem(item string, size uint64, nodeType string) {
	p.items = append(p.items, itemTraceEntry{item, size, nodeType})
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
		printerTraceEntry{State{0, 0, 0, 0}, 0, false},
	}, result)
	test.Equals(t, itemTrace{}, items)
}

func TestAddFileProgress(t *testing.T) {
	fileSize := uint64(100)

	result, items, _ := testProgress(func(progress *Progress) bool {
		progress.AddProgress("test.txt", fileSize, data.NodeTypeFile)
		return false
	})
	test.Equals(t, printerTrace{
		printerTraceEntry{State{1, 0, 1, fileSize}, 0, false},
	}, result)
	test.Equals(t, itemTrace{
		itemTraceEntry{item: "test.txt", size: fileSize, nodeType: "file"},
	}, items)
}

func TestAddDirProgress(t *testing.T) {
	result, items, _ := testProgress(func(progress *Progress) bool {
		progress.AddProgress("test-dir", 0, data.NodeTypeDir)
		return false
	})
	test.Equals(t, printerTrace{
		printerTraceEntry{State{0, 1, 1, 0}, 0, false},
	}, result)
	test.Equals(t, itemTrace{
		itemTraceEntry{item: "test-dir", size: 0, nodeType: "dir"},
	}, items)
}

func TestMultipleItems(t *testing.T) {
	fileSize := uint64(100)

	result, items, _ := testProgress(func(progress *Progress) bool {
		progress.AddProgress("test-dir", 0, data.NodeTypeDir)
		progress.AddProgress("test1.txt", fileSize, data.NodeTypeFile)
		progress.AddProgress("test2.txt", fileSize*2, data.NodeTypeFile)
		return false
	})
	test.Equals(t, printerTrace{
		printerTraceEntry{State{2, 1, 3, fileSize * 3}, 0, false},
	}, result)
	test.Equals(t, itemTrace{
		itemTraceEntry{item: "test-dir", size: 0, nodeType: "dir"},
		itemTraceEntry{item: "test1.txt", size: fileSize, nodeType: "file"},
		itemTraceEntry{item: "test2.txt", size: fileSize * 2, nodeType: "file"},
	}, items)
}

func TestSummaryOnFinish(t *testing.T) {
	fileSize := uint64(100)

	result, _, _ := testProgress(func(progress *Progress) bool {
		progress.AddProgress("test-dir", 0, data.NodeTypeDir)
		progress.AddProgress("test1.txt", fileSize, data.NodeTypeFile)
		progress.AddProgress("test2.txt", fileSize*2, data.NodeTypeFile)
		return true
	})
	test.Equals(t, printerTrace{
		printerTraceEntry{State{2, 1, 3, fileSize * 3}, mockFinishDuration, true},
	}, result)
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

func TestSymlinkProgress(t *testing.T) {
	result, items, _ := testProgress(func(progress *Progress) bool {
		progress.AddProgress("test-symlink", 0, data.NodeTypeSymlink)
		return false
	})
	test.Equals(t, printerTrace{
		printerTraceEntry{State{0, 0, 1, 0}, 0, false},
	}, result)
	test.Equals(t, itemTrace{
		itemTraceEntry{item: "test-symlink", size: 0, nodeType: "symlink"},
	}, items)
}
