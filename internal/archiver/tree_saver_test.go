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

func newFutureBlobWithResponse() FutureBlob {
	ch := make(chan SaveBlobResponse, 1)
	ch <- SaveBlobResponse{
		id:         restic.NewRandomID(),
		known:      false,
		length:     123,
		sizeInRepo: 123,
	}
	return FutureBlob{ch: ch}
}

func TestTreeSaver(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg, ctx := errgroup.WithContext(ctx)

	saveFn := func(ctx context.Context, t restic.BlobType, buf *Buffer) FutureBlob {
		return newFutureBlobWithResponse()
	}
	errFn := func(snPath string, err error) error {
		return err
	}

	b := NewTreeSaver(ctx, wg, uint(runtime.NumCPU()), saveFn, errFn)

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

	b.TriggerShutdown()

	err := wg.Wait()
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
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			wg, ctx := errgroup.WithContext(ctx)

			saveFn := func(ctx context.Context, tpe restic.BlobType, buf *Buffer) FutureBlob {
				return newFutureBlobWithResponse()
			}
			errFn := func(snPath string, err error) error {
				return err
			}

			b := NewTreeSaver(ctx, wg, uint(runtime.NumCPU()), saveFn, errFn)

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

			b.TriggerShutdown()

			err := wg.Wait()
			if err == nil {
				t.Errorf("expected error not found")
			}

			if err != errTest {
				t.Fatalf("unexpected error found: %v", err)
			}
		})
	}
}
