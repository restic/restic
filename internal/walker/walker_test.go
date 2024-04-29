package walker

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/pkg/errors"
	"github.com/restic/restic/internal/restic"
)

// TestTree is used to construct a list of trees for testing the walker.
type TestTree map[string]interface{}

// TestFile is used to test the walker.
type TestFile struct {
	Size uint64
}

func BuildTreeMap(tree TestTree) (m TreeMap, root restic.ID) {
	m = TreeMap{}
	id := buildTreeMap(tree, m)
	return m, id
}

func buildTreeMap(tree TestTree, m TreeMap) restic.ID {
	tb := restic.NewTreeJSONBuilder()
	var names []string
	for name := range tree {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		item := tree[name]
		switch elem := item.(type) {
		case TestFile:
			err := tb.AddNode(&restic.Node{
				Name: name,
				Type: "file",
				Size: elem.Size,
			})
			if err != nil {
				panic(err)
			}
		case TestTree:
			id := buildTreeMap(elem, m)
			err := tb.AddNode(&restic.Node{
				Name:    name,
				Subtree: &id,
				Type:    "dir",
			})
			if err != nil {
				panic(err)
			}
		default:
			panic(fmt.Sprintf("invalid type %T", elem))
		}
	}

	buf, err := tb.Finalize()
	if err != nil {
		panic(err)
	}

	id := restic.Hash(buf)

	if _, ok := m[id]; !ok {
		m[id] = buf
	}

	return id
}

// TreeMap returns the trees from the map on LoadTree.
type TreeMap map[restic.ID][]byte

func (t TreeMap) LoadBlob(_ context.Context, tpe restic.BlobType, id restic.ID, _ []byte) ([]byte, error) {
	if tpe != restic.TreeBlob {
		return nil, errors.New("can only load trees")
	}
	tree, ok := t[id]
	if !ok {
		return nil, errors.New("tree not found")
	}
	return tree, nil
}

func (t TreeMap) Connections() uint {
	return 2
}

// checkFunc returns a function suitable for walking the tree to check
// something, and a function which will check the final result.
type checkFunc func(t testing.TB) (walker WalkFunc, leaveDir func(path string), final func(testing.TB))

// checkItemOrder ensures that the order of the 'path' arguments is the one passed in as 'want'.
func checkItemOrder(want []string) checkFunc {
	pos := 0
	return func(t testing.TB) (walker WalkFunc, leaveDir func(path string), final func(testing.TB)) {
		walker = func(treeID restic.ID, path string, node *restic.Node, err error) error {
			if err != nil {
				t.Errorf("error walking %v: %v", path, err)
				return err
			}

			if pos >= len(want) {
				t.Errorf("additional unexpected path found: %v", path)
				return nil
			}

			if path != want[pos] {
				t.Errorf("wrong path found, want %q, got %q", want[pos], path)
			}
			pos++
			return nil
		}

		leaveDir = func(path string) {
			_ = walker(restic.ID{}, "leave: "+path, nil, nil)
		}

		final = func(t testing.TB) {
			if pos != len(want) {
				t.Errorf("not enough items returned, want %d, got %d", len(want), pos)
			}
		}

		return walker, leaveDir, final
	}
}

// checkParentTreeOrder ensures that the order of the 'parentID' arguments is the one passed in as 'want'.
func checkParentTreeOrder(want []string) checkFunc {
	pos := 0
	return func(t testing.TB) (walker WalkFunc, leaveDir func(path string), final func(testing.TB)) {
		walker = func(treeID restic.ID, path string, node *restic.Node, err error) error {
			if err != nil {
				t.Errorf("error walking %v: %v", path, err)
				return err
			}

			if pos >= len(want) {
				t.Errorf("additional unexpected parent tree ID found: %v", treeID)
				return nil
			}

			if treeID.String() != want[pos] {
				t.Errorf("wrong parent tree ID found, want %q, got %q", want[pos], treeID.String())
			}
			pos++
			return nil
		}

		final = func(t testing.TB) {
			if pos != len(want) {
				t.Errorf("not enough items returned, want %d, got %d", len(want), pos)
			}
		}

		return walker, nil, final
	}
}

// checkSkipFor returns ErrSkipNode if path is in skipFor, it checks that the
// paths the walk func is called for are exactly the ones in wantPaths.
func checkSkipFor(skipFor map[string]struct{}, wantPaths []string) checkFunc {
	var pos int

	return func(t testing.TB) (walker WalkFunc, leaveDir func(path string), final func(testing.TB)) {
		walker = func(treeID restic.ID, path string, node *restic.Node, err error) error {
			if err != nil {
				t.Errorf("error walking %v: %v", path, err)
				return err
			}

			if pos >= len(wantPaths) {
				t.Errorf("additional unexpected path found: %v", path)
				return nil
			}

			if path != wantPaths[pos] {
				t.Errorf("wrong path found, want %q, got %q", wantPaths[pos], path)
			}
			pos++

			if _, ok := skipFor[path]; ok {
				return ErrSkipNode
			}

			return nil
		}

		leaveDir = func(path string) {
			_ = walker(restic.ID{}, "leave: "+path, nil, nil)
		}

		final = func(t testing.TB) {
			if pos != len(wantPaths) {
				t.Errorf("wrong number of paths returned, want %d, got %d", len(wantPaths), pos)
			}
		}

		return walker, leaveDir, final
	}
}

func TestWalker(t *testing.T) {
	var tests = []struct {
		tree   TestTree
		checks []checkFunc
	}{
		{
			tree: TestTree{
				"foo": TestFile{},
				"subdir": TestTree{
					"subfile": TestFile{},
				},
			},
			checks: []checkFunc{
				checkItemOrder([]string{
					"/",
					"/foo",
					"/subdir",
					"/subdir/subfile",
					"leave: /subdir",
					"leave: /",
				}),
				checkParentTreeOrder([]string{
					"a760536a8fd64dd63f8dd95d85d788d71fd1bee6828619350daf6959dcb499a0", // tree /
					"a760536a8fd64dd63f8dd95d85d788d71fd1bee6828619350daf6959dcb499a0", // tree /
					"a760536a8fd64dd63f8dd95d85d788d71fd1bee6828619350daf6959dcb499a0", // tree /
					"670046b44353a89b7cd6ef84c78422232438f10eb225c29c07989ae05283d797", // tree /subdir
				}),
				checkSkipFor(
					map[string]struct{}{
						"/subdir": {},
					}, []string{
						"/",
						"/foo",
						"/subdir",
						"leave: /",
					},
				),
				checkSkipFor(
					map[string]struct{}{
						"/": {},
					}, []string{
						"/",
					},
				),
			},
		},
		{
			tree: TestTree{
				"foo": TestFile{},
				"subdir1": TestTree{
					"subfile1": TestFile{},
				},
				"subdir2": TestTree{
					"subfile2": TestFile{},
					"subsubdir2": TestTree{
						"subsubfile3": TestFile{},
					},
				},
			},
			checks: []checkFunc{
				checkItemOrder([]string{
					"/",
					"/foo",
					"/subdir1",
					"/subdir1/subfile1",
					"leave: /subdir1",
					"/subdir2",
					"/subdir2/subfile2",
					"/subdir2/subsubdir2",
					"/subdir2/subsubdir2/subsubfile3",
					"leave: /subdir2/subsubdir2",
					"leave: /subdir2",
					"leave: /",
				}),
				checkParentTreeOrder([]string{
					"7a0e59b986cc83167d9fbeeefc54e4629770124c5825d391f7ee0598667fcdf1", // tree /
					"7a0e59b986cc83167d9fbeeefc54e4629770124c5825d391f7ee0598667fcdf1", // tree /
					"7a0e59b986cc83167d9fbeeefc54e4629770124c5825d391f7ee0598667fcdf1", // tree /
					"22c9feefa0b9fabc7ec5383c90cfe84ba714babbe4d2968fcb78f0ec7612e82f", // tree /subdir1
					"7a0e59b986cc83167d9fbeeefc54e4629770124c5825d391f7ee0598667fcdf1", // tree /
					"9bfe4aab3ac0ad7a81909355d7221801441fb20f7ed06c0142196b3f10358493", // tree /subdir2
					"9bfe4aab3ac0ad7a81909355d7221801441fb20f7ed06c0142196b3f10358493", // tree /subdir2
					"6b962fef064ef9beecc27dfcd6e0f2e7beeebc9c1f1f4f477d4af59fc45f411d", // tree /subdir2/subsubdir2
				}),
				checkSkipFor(
					map[string]struct{}{
						"/subdir1": {},
					}, []string{
						"/",
						"/foo",
						"/subdir1",
						"/subdir2",
						"/subdir2/subfile2",
						"/subdir2/subsubdir2",
						"/subdir2/subsubdir2/subsubfile3",
						"leave: /subdir2/subsubdir2",
						"leave: /subdir2",
						"leave: /",
					},
				),
				checkSkipFor(
					map[string]struct{}{
						"/subdir1":            {},
						"/subdir2/subsubdir2": {},
					}, []string{
						"/",
						"/foo",
						"/subdir1",
						"/subdir2",
						"/subdir2/subfile2",
						"/subdir2/subsubdir2",
						"leave: /subdir2",
						"leave: /",
					},
				),
				checkSkipFor(
					map[string]struct{}{
						"/foo": {},
					}, []string{
						"/",
						"/foo",
						"leave: /",
					},
				),
			},
		},
		{
			tree: TestTree{
				"foo": TestFile{},
				"subdir1": TestTree{
					"subfile1": TestFile{},
					"subfile2": TestFile{},
					"subfile3": TestFile{},
				},
				"subdir2": TestTree{
					"subfile1": TestFile{},
					"subfile2": TestFile{},
					"subfile3": TestFile{},
				},
				"subdir3": TestTree{
					"subfile1": TestFile{},
					"subfile2": TestFile{},
					"subfile3": TestFile{},
				},
				"zzz other": TestFile{},
			},
			checks: []checkFunc{
				checkItemOrder([]string{
					"/",
					"/foo",
					"/subdir1",
					"/subdir1/subfile1",
					"/subdir1/subfile2",
					"/subdir1/subfile3",
					"leave: /subdir1",
					"/subdir2",
					"/subdir2/subfile1",
					"/subdir2/subfile2",
					"/subdir2/subfile3",
					"leave: /subdir2",
					"/subdir3",
					"/subdir3/subfile1",
					"/subdir3/subfile2",
					"/subdir3/subfile3",
					"leave: /subdir3",
					"/zzz other",
					"leave: /",
				}),
				checkParentTreeOrder([]string{
					"c2efeff7f217a4dfa12a16e8bb3cefedd37c00873605c29e5271c6061030672f", // tree /
					"c2efeff7f217a4dfa12a16e8bb3cefedd37c00873605c29e5271c6061030672f", // tree /
					"c2efeff7f217a4dfa12a16e8bb3cefedd37c00873605c29e5271c6061030672f", // tree /
					"57ee8960c7a86859b090a76e5d013f83d10c0ce11d5460076ca8468706f784ab", // tree /subdir1
					"57ee8960c7a86859b090a76e5d013f83d10c0ce11d5460076ca8468706f784ab", // tree /subdir1
					"57ee8960c7a86859b090a76e5d013f83d10c0ce11d5460076ca8468706f784ab", // tree /subdir1
					"c2efeff7f217a4dfa12a16e8bb3cefedd37c00873605c29e5271c6061030672f", // tree /
					"57ee8960c7a86859b090a76e5d013f83d10c0ce11d5460076ca8468706f784ab", // tree /subdir2
					"57ee8960c7a86859b090a76e5d013f83d10c0ce11d5460076ca8468706f784ab", // tree /subdir2
					"57ee8960c7a86859b090a76e5d013f83d10c0ce11d5460076ca8468706f784ab", // tree /subdir2
					"c2efeff7f217a4dfa12a16e8bb3cefedd37c00873605c29e5271c6061030672f", // tree /
					"57ee8960c7a86859b090a76e5d013f83d10c0ce11d5460076ca8468706f784ab", // tree /subdir3
					"57ee8960c7a86859b090a76e5d013f83d10c0ce11d5460076ca8468706f784ab", // tree /subdir3
					"57ee8960c7a86859b090a76e5d013f83d10c0ce11d5460076ca8468706f784ab", // tree /subdir3
					"c2efeff7f217a4dfa12a16e8bb3cefedd37c00873605c29e5271c6061030672f", // tree /
				}),
			},
		},
		{
			tree: TestTree{
				"subdir1": TestTree{},
				"subdir2": TestTree{},
				"subdir3": TestTree{
					"file": TestFile{},
				},
				"subdir4": TestTree{
					"file": TestFile{},
				},
				"subdir5": TestTree{},
				"subdir6": TestTree{},
			},
			checks: []checkFunc{
				checkItemOrder([]string{
					"/",
					"/subdir1",
					"leave: /subdir1",
					"/subdir2",
					"leave: /subdir2",
					"/subdir3",
					"/subdir3/file",
					"leave: /subdir3",
					"/subdir4",
					"/subdir4/file",
					"leave: /subdir4",
					"/subdir5",
					"leave: /subdir5",
					"/subdir6",
					"leave: /subdir6",
					"leave: /",
				}),
			},
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			repo, root := BuildTreeMap(test.tree)
			for _, check := range test.checks {
				t.Run("", func(t *testing.T) {
					ctx, cancel := context.WithCancel(context.TODO())
					defer cancel()

					fn, leaveDir, last := check(t)
					err := Walk(ctx, repo, root, WalkVisitor{
						ProcessNode: fn,
						LeaveDir:    leaveDir,
					})
					if err != nil {
						t.Error(err)
					}
					last(t)
				})
			}
		})
	}
}
