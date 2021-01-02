package archiver

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	tomb "gopkg.in/tomb.v2"
)

func TestTreeSaver(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tmb, ctx := tomb.WithContext(ctx)

	saveFn := func(context.Context, *restic.Tree) (restic.ID, ItemStats, error) {
		return restic.NewRandomID(), ItemStats{TreeBlobs: 1, TreeSize: 123}, nil
	}

	errFn := func(snPath string, fi os.FileInfo, err error) error {
		return nil
	}

	b := NewTreeSaver(ctx, tmb, uint(runtime.NumCPU()), saveFn, errFn)

	var results []FutureTree

	for i := 0; i < 20; i++ {
		node := &restic.Node{
			Name: fmt.Sprintf("file-%d", i),
		}

		fb := b.Save(ctx, "/", node, nil, nil)
		results = append(results, fb)
	}

	for _, tree := range results {
		tree.Wait(ctx)
	}

	tmb.Kill(nil)

	err := tmb.Wait()
	if err != nil {
		t.Fatal(err)
	}
}

func TestTreeSaverError(t *testing.T) {
	var tests = []struct {
		trees  int
		failAt int32
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

			tmb, ctx := tomb.WithContext(ctx)

			var num int32
			saveFn := func(context.Context, *restic.Tree) (restic.ID, ItemStats, error) {
				val := atomic.AddInt32(&num, 1)
				if val == test.failAt {
					t.Logf("sending error for request %v\n", test.failAt)
					return restic.ID{}, ItemStats{}, errTest
				}
				return restic.NewRandomID(), ItemStats{TreeBlobs: 1, TreeSize: 123}, nil
			}

			errFn := func(snPath string, fi os.FileInfo, err error) error {
				t.Logf("ignoring error %v\n", err)
				return nil
			}

			b := NewTreeSaver(ctx, tmb, uint(runtime.NumCPU()), saveFn, errFn)

			var results []FutureTree

			for i := 0; i < test.trees; i++ {
				node := &restic.Node{
					Name: fmt.Sprintf("file-%d", i),
				}

				fb := b.Save(ctx, "/", node, nil, nil)
				results = append(results, fb)
			}

			for _, tree := range results {
				tree.Wait(ctx)
			}

			tmb.Kill(nil)

			err := tmb.Wait()
			if err == nil {
				t.Errorf("expected error not found")
			}

			if err != errTest {
				t.Fatalf("unexpected error found: %v", err)
			}
		})
	}
}
