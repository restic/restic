package archiver

import (
	"context"
	"fmt"
	"runtime"
	"testing"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/sync/errgroup"
)

func treeSaveHelper(ctx context.Context, t restic.BlobType, buf *Buffer) FutureBlob {
	ch := make(chan SaveBlobResponse, 1)
	ch <- SaveBlobResponse{
		id:         restic.NewRandomID(),
		known:      false,
		length:     len(buf.Data),
		sizeInRepo: len(buf.Data),
	}
	return FutureBlob{ch: ch}
}

func setupTreeSaver() (context.Context, context.CancelFunc, *TreeSaver, func() error) {
	ctx, cancel := context.WithCancel(context.Background())
	wg, ctx := errgroup.WithContext(ctx)

	errFn := func(snPath string, err error) error {
		return err
	}

	b := NewTreeSaver(ctx, wg, uint(runtime.NumCPU()), treeSaveHelper, errFn)

	shutdown := func() error {
		b.TriggerShutdown()
		return wg.Wait()
	}

	return ctx, cancel, b, shutdown
}

func TestTreeSaver(t *testing.T) {
	ctx, cancel, b, shutdown := setupTreeSaver()
	defer cancel()

	var results []FutureNode

	for i := 0; i < 20; i++ {
		node := &restic.Node{
			Name: fmt.Sprintf("file-%d", i),
		}

		fb := b.Save(ctx, join("/", node.Name), node.Name, node, nil, nil)
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

			var results []FutureNode

			for i := 0; i < test.trees; i++ {
				node := &restic.Node{
					Name: fmt.Sprintf("file-%d", i),
				}
				nodes := []FutureNode{
					newFutureNodeWithResult(futureNodeResult{node: &restic.Node{
						Name: fmt.Sprintf("child-%d", i),
					}}),
				}
				if (i + 1) == test.failAt {
					nodes = append(nodes, newFutureNodeWithResult(futureNodeResult{
						err: errTest,
					}))
				}

				fb := b.Save(ctx, join("/", node.Name), node.Name, node, nodes, nil)
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
