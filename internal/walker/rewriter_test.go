package walker

import (
	"context"
	"slices"
	"testing"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
)

type checkRewriteFunc func(t testing.TB) (rewriter *TreeRewriter, final func(testing.TB))

// checkRewriteItemOrder ensures that the order of the 'path' arguments is the one passed in as 'want'.
func checkRewriteItemOrder(want []string) checkRewriteFunc {
	pos := 0
	return func(t testing.TB) (rewriter *TreeRewriter, final func(testing.TB)) {
		rewriter = NewTreeRewriter(RewriteOpts{
			RewriteNode: func(node *data.Node, path string) *data.Node {
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
			RewriteNode: func(node *data.Node, path string) *data.Node {
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
			RewriteNode: func(node *data.Node, path string) *data.Node {
				if node.Type == data.NodeTypeFile {
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
			modrepo := data.TestWritableTreeMap{TestTreeMap: repo}

			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			rewriter, last := test.check(t)
			newRoot, err := rewriter.RewriteTree(ctx, modrepo, modrepo, "/", root)
			if err != nil {
				t.Error(err)
			}
			last(t)

			// verifying against the expected tree root also implicitly checks the structural integrity
			if newRoot != expRoot {
				t.Error("hash mismatch")
				t.Log("Got")
				modrepo.Dump(t)
				t.Log("Expected")
				data.TestWritableTreeMap{TestTreeMap: expRepo}.Dump(t)
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
		modrepo := data.TestWritableTreeMap{TestTreeMap: repo}

		ctx, cancel := context.WithCancel(context.TODO())
		defer cancel()

		rewriteNode := func(node *data.Node, path string) *data.Node {
			if path == "/bar" {
				return nil
			}
			if node.Type == data.NodeTypeFile {
				node.Size += 21
			}
			return node
		}
		rewriter, querySize := NewSnapshotSizeRewriter(rewriteNode, nil)
		newRoot, err := rewriter.RewriteTree(ctx, modrepo, modrepo, "/", root)
		if err != nil {
			t.Error(err)
		}

		ss := querySize()

		test.Equals(t, uint(2), ss.FileCount, "snapshot file count mismatch")
		test.Equals(t, uint64(84), ss.FileSize, "snapshot size mismatch")

		// verifying against the expected tree root also implicitly checks the structural integrity
		if newRoot != expRoot {
			t.Error("hash mismatch")
			t.Log("Got")
			modrepo.Dump(t)
			t.Log("Expected")
			data.TestWritableTreeMap{TestTreeMap: expRepo}.Dump(t)
		}
	})

}

func TestRewriterKeepEmptyDirectory(t *testing.T) {
	var paths []string
	tests := []struct {
		name      string
		keepEmpty NodeKeepEmptyDirectoryFunc
		assert    func(t *testing.T, newRoot restic.ID)
	}{
		{
			name:      "Keep",
			keepEmpty: func(string) bool { return true },
			assert: func(t *testing.T, newRoot restic.ID) {
				_, expRoot := BuildTreeMap(TestTree{"empty": TestTree{}})
				test.Assert(t, newRoot == expRoot, "expected empty dir kept")
			},
		},
		{
			name:      "Drop subdir only",
			keepEmpty: func(p string) bool { return p != "/empty" },
			assert: func(t *testing.T, newRoot restic.ID) {
				_, expRoot := BuildTreeMap(TestTree{})
				test.Assert(t, newRoot == expRoot, "expected empty root")
			},
		},
		{
			name:      "Drop all",
			keepEmpty: func(string) bool { return false },
			assert: func(t *testing.T, newRoot restic.ID) {
				test.Assert(t, newRoot.IsNull(), "expected null root")
			},
		},
		{
			name: "Paths",
			keepEmpty: func(p string) bool {
				paths = append(paths, p)
				return p != "/empty"
			},
			assert: func(t *testing.T, newRoot restic.ID) {
				test.Assert(t, len(paths) >= 2, "expected at least two KeepEmptyDirectory calls")
				var hasRoot, hasEmpty bool
				for _, p := range paths {
					if p == "/" {
						hasRoot = true
					}
					if p == "/empty" {
						hasEmpty = true
					}
				}
				test.Assert(t, hasRoot && hasEmpty, "expected paths \"/\" and \"/empty\", got %v", paths)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			repo, root := BuildTreeMap(TestTree{"empty": TestTree{}})
			modrepo := data.TestWritableTreeMap{TestTreeMap: repo}

			rw := NewTreeRewriter(RewriteOpts{KeepEmptyDirectory: tc.keepEmpty})
			newRoot, err := rw.RewriteTree(ctx, modrepo, modrepo, "/", root)
			test.OK(t, err)
			tc.assert(t, newRoot)
		})
	}
}

func TestRewriterFailOnUnknownFields(t *testing.T) {
	tm := data.TestWritableTreeMap{TestTreeMap: data.TestTreeMap{}}
	node := []byte(`{"nodes":[{"name":"subfile","type":"file","mtime":"0001-01-01T00:00:00Z","atime":"0001-01-01T00:00:00Z","ctime":"0001-01-01T00:00:00Z","uid":0,"gid":0,"content":null,"unknown_field":42}]}`)
	id := restic.Hash(node)
	tm.TestTreeMap[id] = node

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	rewriter := NewTreeRewriter(RewriteOpts{
		RewriteNode: func(node *data.Node, path string) *data.Node {
			// tree loading must not succeed
			t.Fail()
			return node
		},
	})
	_, err := rewriter.RewriteTree(ctx, tm, tm, "/", id)

	if err == nil {
		t.Error("missing error on unknown field")
	}

	// check that the serialization check can be disabled
	rewriter = NewTreeRewriter(RewriteOpts{
		AllowUnstableSerialization: true,
	})
	root, err := rewriter.RewriteTree(ctx, tm, tm, "/", id)
	test.OK(t, err)
	_, expRoot := BuildTreeMap(TestTree{
		"subfile": TestFile{},
	})
	test.Assert(t, root == expRoot, "mismatched trees")
}

func TestRewriterTreeLoadError(t *testing.T) {
	tm := data.TestWritableTreeMap{TestTreeMap: data.TestTreeMap{}}
	id := restic.NewRandomID()

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	// also check that load error by default cause the operation to fail
	rewriter := NewTreeRewriter(RewriteOpts{})
	_, err := rewriter.RewriteTree(ctx, tm, tm, "/", id)
	if err == nil {
		t.Fatal("missing error on unloadable tree")
	}

	replacementNode := &data.Node{Name: "replacement", Type: data.NodeTypeFile, Size: 42}
	replacementID := data.TestSaveNodes(t, ctx, tm, []*data.Node{replacementNode})

	rewriter = NewTreeRewriter(RewriteOpts{
		RewriteFailedTree: func(nodeID restic.ID, path string, err error) (data.TreeNodeIterator, error) {
			if nodeID != id || path != "/" {
				t.Fail()
			}
			return slices.Values([]data.NodeOrError{{Node: replacementNode}}), nil
		},
	})
	newRoot, err := rewriter.RewriteTree(ctx, tm, tm, "/", id)
	test.OK(t, err)
	test.Equals(t, replacementID, newRoot)
}
