package archiver

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"testing"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
	"golang.org/x/sync/errgroup"
)

type mockSaver struct {
	saved map[string]int
	mutex sync.Mutex
}

func (m *mockSaver) SaveBlobAsync(_ context.Context, _ restic.BlobType, buf []byte, id restic.ID, storeDuplicate bool, cb func(newID restic.ID, known bool, sizeInRepo int, err error)) {
	// Fake async operation
	go func() {
		m.mutex.Lock()
		m.saved[string(buf)]++
		m.mutex.Unlock()

		cb(restic.Hash(buf), false, len(buf), nil)
	}()
}

func (m *mockSaver) SaveBlobFromReaderAsync(_ context.Context, _ restic.BlobType, rd io.ReadSeeker, size int64, cb func(newID restic.ID, known bool, sizeInRepo int, err error)) {
	// Fake async operation
	go func() {
		buf, err := io.ReadAll(rd)
		if err != nil {
			cb(restic.ID{}, false, 0, err)
			return
		}
		if int64(len(buf)) != size {
			cb(restic.ID{}, false, 0, fmt.Errorf("got %d bytes, expected %d", len(buf), size))
			return
		}

		m.mutex.Lock()
		m.saved[string(buf)]++
		m.mutex.Unlock()

		cb(restic.Hash(buf), false, len(buf), nil)
	}()
}

func setupTreeSaver() (context.Context, context.CancelFunc, *treeSaver, func() error) {
	ctx, cancel := context.WithCancel(context.Background())
	wg, ctx := errgroup.WithContext(ctx)

	errFn := func(snPath string, err error) error {
		return err
	}

	b := newTreeSaver(ctx, wg, uint(runtime.NumCPU()), &mockSaver{saved: make(map[string]int)}, errFn)

	shutdown := func() error {
		b.TriggerShutdown()
		return wg.Wait()
	}

	return ctx, cancel, b, shutdown
}

func TestTreeSaver(t *testing.T) {
	ctx, cancel, b, shutdown := setupTreeSaver()
	defer cancel()

	var results []futureNode

	for i := range 20 {
		node := &data.Node{
			Name: fmt.Sprintf("file-%d", i),
		}

		fb := b.Save(ctx, join("/", node.Name), node.Name, node, nil, nil, nil)
		results = append(results, fb)
	}

	for _, tree := range results {
		tree.take(ctx)
	}

	err := shutdown()
	if err != nil {
		t.Fatal(err)
	}
}

func TestTreeSaverError(t *testing.T) {
	var tests = []struct {
		trees  int
		failAt int
	}{
		{1, 1},
		{20, 2},
		{20, 5},
		{20, 15},
		{200, 150},
	}

	errTest := errors.New("test error")

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			ctx, cancel, b, shutdown := setupTreeSaver()
			defer cancel()

			var results []futureNode

			for i := 0; i < test.trees; i++ {
				node := &data.Node{
					Name: fmt.Sprintf("file-%d", i),
				}
				nodes := []futureNode{
					newFutureNodeWithResult(futureNodeResult{node: &data.Node{
						Name: fmt.Sprintf("child-%d", i),
					}}),
				}
				if (i + 1) == test.failAt {
					nodes = append(nodes, newFutureNodeWithResult(futureNodeResult{
						err: errTest,
					}))
				}

				fb := b.Save(ctx, join("/", node.Name), node.Name, node, nil, nodes, nil)
				results = append(results, fb)
			}

			for _, tree := range results {
				tree.take(ctx)
			}

			err := shutdown()
			if err == nil {
				t.Errorf("expected error not found")
			}
			if err != errTest {
				t.Fatalf("unexpected error found: %v", err)
			}
		})
	}
}

func TestTreeSaverDuplicates(t *testing.T) {
	for _, identicalNodes := range []bool{true, false} {
		t.Run("", func(t *testing.T) {
			ctx, cancel, b, shutdown := setupTreeSaver()
			defer cancel()

			node := &data.Node{
				Name: "file",
			}
			nodes := []futureNode{
				newFutureNodeWithResult(futureNodeResult{node: &data.Node{
					Name: "child",
				}}),
			}
			if identicalNodes {
				nodes = append(nodes, newFutureNodeWithResult(futureNodeResult{node: &data.Node{
					Name: "child",
				}}))
			} else {
				nodes = append(nodes, newFutureNodeWithResult(futureNodeResult{node: &data.Node{
					Name: "child",
					Size: 42,
				}}))
			}

			fb := b.Save(ctx, join("/", node.Name), node.Name, node, nil, nodes, nil)
			fb.take(ctx)

			err := shutdown()
			if identicalNodes {
				test.Assert(t, err == nil, "unexpected error found: %v", err)
			} else {
				test.Assert(t, err != nil, "expected error not found")
			}
		})
	}
}

// slowReaderSaver hands the reader to a goroutine that does not touch it until
// told to, which is what lets a test cancel the backup in between.
type slowReaderSaver struct {
	mockSaver
	gotReader chan struct{}
	proceed   chan struct{}
	readErr   chan error
}

func (m *slowReaderSaver) SaveBlobFromReaderAsync(_ context.Context, _ restic.BlobType, rd io.ReadSeeker, _ int64, cb func(newID restic.ID, known bool, sizeInRepo int, err error)) {
	go func() {
		m.gotReader <- struct{}{}
		<-m.proceed
		_, err := io.ReadAll(rd)
		m.readErr <- err
		cb(restic.ID{}, false, 0, err)
	}()
}

// TestTreeSaverSpilledTreeSurvivesCancel covers a tree written to a temp file
// whose backup is cancelled while the repository still has the file. Cancelling
// must not pull the file out from under the upload that is reading it: the read
// either finishes or fails for a reason of its own, never because the file was
// closed early.
func TestTreeSaverSpilledTreeSurvivesCancel(t *testing.T) {
	saver := &slowReaderSaver{
		mockSaver: mockSaver{saved: make(map[string]int)},
		gotReader: make(chan struct{}),
		proceed:   make(chan struct{}),
		readErr:   make(chan error, 1),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wg, ctx := errgroup.WithContext(ctx)
	errFn := func(_ string, err error) error { return err }
	b := newTreeSaver(ctx, wg, 1, saver, errFn)

	// Asking for a directory this wide is what makes the builder spill to a
	// temp file, without having to feed it that many nodes.
	builder := newTreeBuilder(errFn, spillTreeEntries)
	test.Assert(t, builder.spill != nil, "the builder did not spill to a temp file")
	test.OK(t, builder.add(futureNodeResult{node: &data.Node{Name: "a", Type: data.NodeTypeFile}}))

	node := &data.Node{Name: "dir", Type: data.NodeTypeDir}
	fb := b.Save(ctx, "/dir", "dir", node, builder, nil, nil)

	// Wait until the repository holds the reader, then cancel and let the
	// tree saver return before the read happens.
	<-saver.gotReader
	cancel()
	fb.take(context.Background())

	saver.proceed <- struct{}{}
	err := <-saver.readErr
	test.Assert(t, !errors.Is(err, os.ErrClosed),
		"the temp file was closed while the repository was still reading it: %v", err)
	test.OK(t, err)

	b.TriggerShutdown()
	_ = wg.Wait()
}
