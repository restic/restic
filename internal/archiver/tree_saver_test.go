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
	rtest "github.com/restic/restic/internal/test"
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

func safeTreeInsert(t *testing.T, tree *restic.Tree, node *restic.Node) {
	rtest.OK(t, tree.Insert(node))
}

// Assert that TreeSaver JSON output is exactly what encoding/json produces,
// regardless of the actual JSON serializer.
func TestTreeSaverJSON(t *testing.T) {
	tree := restic.NewTree(3)
	safeTreeInsert(t, tree, &restic.Node{Name: "foo.txt", Type: "file", Device: 1, Path: "/foo.txt"})
	safeTreeInsert(t, tree, &restic.Node{Name: "bar.txt", Type: "file", Device: 1})

	subtree := restic.NewTree(2)
	safeTreeInsert(t, subtree, &restic.Node{Name: "foo.txt", Type: "file", Device: 2})
	safeTreeInsert(t, subtree, &restic.Node{Name: "bar.txt", Type: "file", Device: 2})
	safeTreeInsert(t, tree, &restic.Node{Name: "subdir", Type: "dir", Subtree: &restic.ID{}})

	buf, err := treeJSON(tree)
	rtest.OK(t, err)

	expect := `{"nodes":[{"name":"bar.txt","type":"file","mtime":"0001-01-01T00:00:00Z","atime":"0001-01-01T00:00:00Z","ctime":"0001-01-01T00:00:00Z","uid":0,"gid":0,"device":1,"content":null},{"name":"foo.txt","type":"file","mtime":"0001-01-01T00:00:00Z","atime":"0001-01-01T00:00:00Z","ctime":"0001-01-01T00:00:00Z","uid":0,"gid":0,"device":1,"content":null},{"name":"subdir","type":"dir","mtime":"0001-01-01T00:00:00Z","atime":"0001-01-01T00:00:00Z","ctime":"0001-01-01T00:00:00Z","uid":0,"gid":0,"content":null,"subtree":"0000000000000000000000000000000000000000000000000000000000000000"}]}` + "\n"

	rtest.Equals(t, expect, string(buf))
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
