package walker

import (
	"context"
	"fmt"
	"testing"

	"github.com/pkg/errors"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
)

// WritableTreeMap also support saving
type WritableTreeMap struct {
	TreeMap
}

func (t WritableTreeMap) SaveBlob(_ context.Context, tpe restic.BlobType, buf []byte, id restic.ID, _ bool) (newID restic.ID, known bool, size int, err error) {
	if tpe != restic.TreeBlob {
		return restic.ID{}, false, 0, errors.New("can only save trees")
	}

	if id.IsNull() {
		id = restic.Hash(buf)
	}
	_, ok := t.TreeMap[id]
	if ok {
		return id, false, 0, nil
	}

	t.TreeMap[id] = append([]byte{}, buf...)
	return id, true, len(buf), nil
}

func (t WritableTreeMap) Dump() {
	for k, v := range t.TreeMap {
		fmt.Printf("%v: %v", k, string(v))
	}
}

type checkRewriteFunc func(t testing.TB) (rewriter *TreeRewriter, final func(testing.TB))

// checkRewriteItemOrder ensures that the order of the 'path' arguments is the one passed in as 'want'.
func checkRewriteItemOrder(want []string) checkRewriteFunc {
	pos := 0
	return func(t testing.TB) (rewriter *TreeRewriter, final func(testing.TB)) {
		rewriter = NewTreeRewriter(RewriteOpts{
			RewriteNode: func(node *restic.Node, path string) *restic.Node {
				if pos >= len(want) {
					t.Errorf("additional unexpected path found: %v", path)
					return nil
				}

				if path != want[pos] {
					t.Errorf("wrong path found, want %q, got %q", want[pos], path)
				}
				pos++
				return node
			},
		})

		final = func(t testing.TB) {
			if pos != len(want) {
				t.Errorf("not enough items returned, want %d, got %d", len(want), pos)
			}
		}

		return rewriter, final
	}
}

// checkRewriteSkips excludes nodes if path is in skipFor, it checks that rewriting proceeds in the correct order.
func checkRewriteSkips(skipFor map[string]struct{}, want []string, disableCache bool) checkRewriteFunc {
	var pos int

	return func(t testing.TB) (rewriter *TreeRewriter, final func(testing.TB)) {
		rewriter = NewTreeRewriter(RewriteOpts{
			RewriteNode: func(node *restic.Node, path string) *restic.Node {
				if pos >= len(want) {
					t.Errorf("additional unexpected path found: %v", path)
					return nil
				}

				if path != want[pos] {
					t.Errorf("wrong path found, want %q, got %q", want[pos], path)
				}
				pos++

				_, skip := skipFor[path]
				if skip {
					return nil
				}
				return node
			},
			DisableNodeCache: disableCache,
		})

		final = func(t testing.TB) {
			if pos != len(want) {
				t.Errorf("not enough items returned, want %d, got %d", len(want), pos)
			}
		}

		return rewriter, final
	}
}

// checkIncreaseNodeSize modifies each node by changing its size.
func checkIncreaseNodeSize(increase uint64) checkRewriteFunc {
	return func(t testing.TB) (rewriter *TreeRewriter, final func(testing.TB)) {
		rewriter = NewTreeRewriter(RewriteOpts{
			RewriteNode: func(node *restic.Node, path string) *restic.Node {
				if node.Type == "file" {
					node.Size += increase
				}
				return node
			},
		})

		final = func(t testing.TB) {}

		return rewriter, final
	}
}

func TestRewriter(t *testing.T) {
	var tests = []struct {
		tree    TestTree
		newTree TestTree
		check   checkRewriteFunc
	}{
		{ // don't change
			tree: TestTree{
				"foo": TestFile{},
				"subdir": TestTree{
					"subfile": TestFile{},
				},
			},
			check: checkRewriteItemOrder([]string{
				"/foo",
				"/subdir",
				"/subdir/subfile",
			}),
		},
		{ // exclude file
			tree: TestTree{
				"foo": TestFile{},
				"subdir": TestTree{
					"subfile": TestFile{},
				},
			},
			newTree: TestTree{
				"foo":    TestFile{},
				"subdir": TestTree{},
			},
			check: checkRewriteSkips(
				map[string]struct{}{
					"/subdir/subfile": {},
				},
				[]string{
					"/foo",
					"/subdir",
					"/subdir/subfile",
				},
				false,
			),
		},
		{ // exclude dir
			tree: TestTree{
				"foo": TestFile{},
				"subdir": TestTree{
					"subfile": TestFile{},
				},
			},
			newTree: TestTree{
				"foo": TestFile{},
			},
			check: checkRewriteSkips(
				map[string]struct{}{
					"/subdir": {},
				},
				[]string{
					"/foo",
					"/subdir",
				},
				false,
			),
		},
		{ // modify node
			tree: TestTree{
				"foo": TestFile{Size: 21},
				"subdir": TestTree{
					"subfile": TestFile{Size: 21},
				},
			},
			newTree: TestTree{
				"foo": TestFile{Size: 42},
				"subdir": TestTree{
					"subfile": TestFile{Size: 42},
				},
			},
			check: checkIncreaseNodeSize(21),
		},
		{ // test cache
			tree: TestTree{
				// both subdirs are identical
				"subdir1": TestTree{
					"subfile":  TestFile{},
					"subfile2": TestFile{},
				},
				"subdir2": TestTree{
					"subfile":  TestFile{},
					"subfile2": TestFile{},
				},
			},
			newTree: TestTree{
				"subdir1": TestTree{
					"subfile2": TestFile{},
				},
				"subdir2": TestTree{
					"subfile2": TestFile{},
				},
			},
			check: checkRewriteSkips(
				map[string]struct{}{
					"/subdir1/subfile": {},
				},
				[]string{
					"/subdir1",
					"/subdir1/subfile",
					"/subdir1/subfile2",
					"/subdir2",
				},
				false,
			),
		},
		{ // test disabled cache
			tree: TestTree{
				// both subdirs are identical
				"subdir1": TestTree{
					"subfile":  TestFile{},
					"subfile2": TestFile{},
				},
				"subdir2": TestTree{
					"subfile":  TestFile{},
					"subfile2": TestFile{},
				},
			},
			newTree: TestTree{
				"subdir1": TestTree{
					"subfile2": TestFile{},
				},
				"subdir2": TestTree{
					"subfile":  TestFile{},
					"subfile2": TestFile{},
				},
			},
			check: checkRewriteSkips(
				map[string]struct{}{
					"/subdir1/subfile": {},
				},
				[]string{
					"/subdir1",
					"/subdir1/subfile",
					"/subdir1/subfile2",
					"/subdir2",
					"/subdir2/subfile",
					"/subdir2/subfile2",
				},
				true,
			),
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			repo, root := BuildTreeMap(test.tree)
			if test.newTree == nil {
				test.newTree = test.tree
			}
			expRepo, expRoot := BuildTreeMap(test.newTree)
			modrepo := WritableTreeMap{repo}

			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			rewriter, last := test.check(t)
			newRoot, err := rewriter.RewriteTree(ctx, modrepo, "/", root)
			if err != nil {
				t.Error(err)
			}
			last(t)

			// verifying against the expected tree root also implicitly checks the structural integrity
			if newRoot != expRoot {
				t.Error("hash mismatch")
				fmt.Println("Got")
				modrepo.Dump()
				fmt.Println("Expected")
				WritableTreeMap{expRepo}.Dump()
			}
		})
	}
}

func TestSnapshotSizeQuery(t *testing.T) {
	tree := TestTree{
		"foo": TestFile{Size: 21},
		"bar": TestFile{Size: 21},
		"subdir": TestTree{
			"subfile": TestFile{Size: 21},
		},
	}
	newTree := TestTree{
		"foo": TestFile{Size: 42},
		"subdir": TestTree{
			"subfile": TestFile{Size: 42},
		},
	}
	t.Run("", func(t *testing.T) {
		repo, root := BuildTreeMap(tree)
		expRepo, expRoot := BuildTreeMap(newTree)
		modrepo := WritableTreeMap{repo}

		ctx, cancel := context.WithCancel(context.TODO())
		defer cancel()

		rewriteNode := func(node *restic.Node, path string) *restic.Node {
			if path == "/bar" {
				return nil
			}
			if node.Type == "file" {
				node.Size += 21
			}
			return node
		}
		rewriter, querySize := NewSnapshotSizeRewriter(rewriteNode)
		newRoot, err := rewriter.RewriteTree(ctx, modrepo, "/", root)
		if err != nil {
			t.Error(err)
		}

		ss := querySize()

		test.Equals(t, uint(2), ss.FileCount, "snapshot file count mismatch")
		test.Equals(t, uint64(84), ss.FileSize, "snapshot size mismatch")

		// verifying against the expected tree root also implicitly checks the structural integrity
		if newRoot != expRoot {
			t.Error("hash mismatch")
			fmt.Println("Got")
			modrepo.Dump()
			fmt.Println("Expected")
			WritableTreeMap{expRepo}.Dump()
		}
	})

}

func TestRewriterFailOnUnknownFields(t *testing.T) {
	tm := WritableTreeMap{TreeMap{}}
	node := []byte(`{"nodes":[{"name":"subfile","type":"file","mtime":"0001-01-01T00:00:00Z","atime":"0001-01-01T00:00:00Z","ctime":"0001-01-01T00:00:00Z","uid":0,"gid":0,"content":null,"unknown_field":42}]}`)
	id := restic.Hash(node)
	tm.TreeMap[id] = node

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	rewriter := NewTreeRewriter(RewriteOpts{
		RewriteNode: func(node *restic.Node, path string) *restic.Node {
			// tree loading must not succeed
			t.Fail()
			return node
		},
	})
	_, err := rewriter.RewriteTree(ctx, tm, "/", id)

	if err == nil {
		t.Error("missing error on unknown field")
	}

	// check that the serialization check can be disabled
	rewriter = NewTreeRewriter(RewriteOpts{
		AllowUnstableSerialization: true,
	})
	root, err := rewriter.RewriteTree(ctx, tm, "/", id)
	test.OK(t, err)
	_, expRoot := BuildTreeMap(TestTree{
		"subfile": TestFile{},
	})
	test.Assert(t, root == expRoot, "mismatched trees")
}

func TestRewriterTreeLoadError(t *testing.T) {
	tm := WritableTreeMap{TreeMap{}}
	id := restic.NewRandomID()

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	// also check that load error by default cause the operation to fail
	rewriter := NewTreeRewriter(RewriteOpts{})
	_, err := rewriter.RewriteTree(ctx, tm, "/", id)
	if err == nil {
		t.Fatal("missing error on unloadable tree")
	}

	replacementID := restic.NewRandomID()
	rewriter = NewTreeRewriter(RewriteOpts{
		RewriteFailedTree: func(nodeID restic.ID, path string, err error) (restic.ID, error) {
			if nodeID != id || path != "/" {
				t.Fail()
			}
			return replacementID, nil
		},
	})
	newRoot, err := rewriter.RewriteTree(ctx, tm, "/", id)
	test.OK(t, err)
	test.Equals(t, replacementID, newRoot)
}
