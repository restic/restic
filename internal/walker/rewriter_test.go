package walker

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/restic/restic/internal/restic"
)

// WritableTreeMap also support saving
type WritableTreeMap struct {
	TreeMap
}

func (t WritableTreeMap) SaveBlob(ctx context.Context, tpe restic.BlobType, buf []byte, id restic.ID, storeDuplicate bool) (newID restic.ID, known bool, size int, err error) {
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

type checkRewriteFunc func(t testing.TB) (visitor TreeFilterVisitor, final func(testing.TB))

// checkRewriteItemOrder ensures that the order of the 'path' arguments is the one passed in as 'want'.
func checkRewriteItemOrder(want []string) checkRewriteFunc {
	pos := 0
	return func(t testing.TB) (visitor TreeFilterVisitor, final func(testing.TB)) {
		vis := TreeFilterVisitor{
			SelectByName: func(path string) bool {
				if pos >= len(want) {
					t.Errorf("additional unexpected path found: %v", path)
					return false
				}

				if path != want[pos] {
					t.Errorf("wrong path found, want %q, got %q", want[pos], path)
				}
				pos++
				return true
			},
		}

		final = func(t testing.TB) {
			if pos != len(want) {
				t.Errorf("not enough items returned, want %d, got %d", len(want), pos)
			}
		}

		return vis, final
	}
}

// checkRewriteSkips excludes nodes if path is in skipFor, it checks that all excluded entries are printed.
func checkRewriteSkips(skipFor map[string]struct{}, want []string) checkRewriteFunc {
	var pos int
	printed := make(map[string]struct{})

	return func(t testing.TB) (visitor TreeFilterVisitor, final func(testing.TB)) {
		vis := TreeFilterVisitor{
			SelectByName: func(path string) bool {
				if pos >= len(want) {
					t.Errorf("additional unexpected path found: %v", path)
					return false
				}

				if path != want[pos] {
					t.Errorf("wrong path found, want %q, got %q", want[pos], path)
				}
				pos++

				_, ok := skipFor[path]
				return !ok
			},
			PrintExclude: func(s string) {
				if _, ok := printed[s]; ok {
					t.Errorf("path was already printed %v", s)
				}
				printed[s] = struct{}{}
			},
		}

		final = func(t testing.TB) {
			if !cmp.Equal(skipFor, printed) {
				t.Errorf("unexpected paths skipped: %s", cmp.Diff(skipFor, printed))
			}
			if pos != len(want) {
				t.Errorf("not enough items returned, want %d, got %d", len(want), pos)
			}
		}

		return vis, final
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

			vis, last := test.check(t)
			newRoot, err := FilterTree(ctx, modrepo, "/", root, &vis)
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

func TestRewriterFailOnUnknownFields(t *testing.T) {
	tm := WritableTreeMap{TreeMap{}}
	node := []byte(`{"nodes":[{"name":"subfile","type":"file","mtime":"0001-01-01T00:00:00Z","atime":"0001-01-01T00:00:00Z","ctime":"0001-01-01T00:00:00Z","uid":0,"gid":0,"content":null,"unknown_field":42}]}`)
	id := restic.Hash(node)
	tm.TreeMap[id] = node

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	// use nil visitor to crash if the tree loading works unexpectedly
	_, err := FilterTree(ctx, tm, "/", id, nil)

	if err == nil {
		t.Error("missing error on unknown field")
	}
}
