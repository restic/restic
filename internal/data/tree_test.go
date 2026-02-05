package data_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/restic/restic/internal/archiver"
	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

var testFiles = []struct {
	name    string
	content []byte
}{
	{"foo", []byte("bar")},
	{"bar/foo2", []byte("bar2")},
	{"bar/bla/blubb", []byte("This is just a test!\n")},
}

func createTempDir(t *testing.T) string {
	tempdir, err := os.MkdirTemp(rtest.TestTempDir, "restic-test-")
	rtest.OK(t, err)

	for _, test := range testFiles {
		file := filepath.Join(tempdir, test.name)
		dir := filepath.Dir(file)
		if dir != "." {
			rtest.OK(t, os.MkdirAll(dir, 0755))
		}

		f, err := os.Create(file)
		defer func() {
			rtest.OK(t, f.Close())
		}()

		rtest.OK(t, err)

		_, err = f.Write(test.content)
		rtest.OK(t, err)
	}

	return tempdir
}

func TestTree(t *testing.T) {
	dir := createTempDir(t)
	defer func() {
		if rtest.TestCleanupTempDirs {
			rtest.RemoveAll(t, dir)
		}
	}()
}

var testNodes = []data.Node{
	{Name: "normal"},
	{Name: "with backslashes \\zzz"},
	{Name: "test utf-8 föbärß"},
	{Name: "test invalid \x00\x01\x02\x03\x04"},
	{Name: "test latin1 \x75\x6d\x6c\xe4\xfc\x74\xf6\x6e\xdf\x6e\x6c\x6c"},
}

func TestNodeMarshal(t *testing.T) {
	for i, n := range testNodes {
		nodeData, err := json.Marshal(&n)
		rtest.OK(t, err)

		var node data.Node
		err = json.Unmarshal(nodeData, &node)
		rtest.OK(t, err)

		if n.Name != node.Name {
			t.Fatalf("Node %d: Names are not equal, want: %q got: %q", i, n.Name, node.Name)
		}
	}
}

func nodeForFile(t *testing.T, name string) *data.Node {
	f, err := (&fs.Local{}).OpenFile(name, fs.O_NOFOLLOW, true)
	rtest.OK(t, err)
	node, err := f.ToNode(false, t.Logf)
	rtest.OK(t, err)
	rtest.OK(t, f.Close())
	return node
}

func TestNodeComparison(t *testing.T) {
	node := nodeForFile(t, "tree_test.go")

	n2 := *node
	rtest.Assert(t, node.Equals(n2), "nodes aren't equal")

	n2.Size--
	rtest.Assert(t, !node.Equals(n2), "nodes are equal")
}

func TestEmptyLoadTree(t *testing.T) {
	repo := repository.TestRepository(t)

	nodes := []*data.Node{}
	var id restic.ID
	rtest.OK(t, repo.WithBlobUploader(context.TODO(), func(ctx context.Context, uploader restic.BlobSaverWithAsync) error {
		// save tree
		id = data.TestSaveNodes(t, ctx, uploader, nodes)
		return nil
	}))

	// load tree again
	it, err := data.LoadTree(context.TODO(), repo, id)
	rtest.OK(t, err)
	nodes2 := []*data.Node{}
	for item := range it {
		rtest.OK(t, item.Error)
		nodes2 = append(nodes2, item.Node)
	}

	rtest.Assert(t, slices.Equal(nodes, nodes2),
		"tree nodes are not equal: want %v, got %v",
		nodes, nodes2)
}

// Basic type for comparing the serialization of the tree
type Tree struct {
	Nodes []*data.Node `json:"nodes"`
}

func TestTreeEqualSerialization(t *testing.T) {
	files := []string{"node.go", "tree.go", "tree_test.go"}
	for i := 1; i <= len(files); i++ {
		tree := Tree{Nodes: make([]*data.Node, 0, i)}
		builder := data.NewTreeJSONBuilder()

		for _, fn := range files[:i] {
			node := nodeForFile(t, fn)

			tree.Nodes = append(tree.Nodes, node)
			rtest.OK(t, builder.AddNode(node))

			rtest.Assert(t, builder.AddNode(node) != nil, "no error on duplicate node")
			rtest.Assert(t, errors.Is(builder.AddNode(node), data.ErrTreeNotOrdered), "wrong error returned")
		}

		treeBytes, err := json.Marshal(tree)
		treeBytes = append(treeBytes, '\n')
		rtest.OK(t, err)

		buf, err := builder.Finalize()
		rtest.OK(t, err)

		// compare serialization of an individual node and the SaveTreeIterator
		rtest.Equals(t, treeBytes, buf)
	}
}

func TestTreeLoadSaveCycle(t *testing.T) {
	files := []string{"node.go", "tree.go", "tree_test.go"}
	builder := data.NewTreeJSONBuilder()
	for _, fn := range files {
		node := nodeForFile(t, fn)
		rtest.OK(t, builder.AddNode(node))
	}
	buf, err := builder.Finalize()
	rtest.OK(t, err)

	tm := data.TestTreeMap{restic.Hash(buf): buf}
	it, err := data.LoadTree(context.TODO(), tm, restic.Hash(buf))
	rtest.OK(t, err)

	mtm := data.TestWritableTreeMap{TestTreeMap: data.TestTreeMap{}}
	id, err := data.SaveTree(context.TODO(), mtm, it)
	rtest.OK(t, err)
	rtest.Equals(t, restic.Hash(buf), id, "saved tree id mismatch")
}

func BenchmarkBuildTree(b *testing.B) {
	const size = 100 // Directories of this size are not uncommon.

	nodes := make([]data.Node, size)
	for i := range nodes {
		// Archiver.SaveTree inputs in sorted order, so do that here too.
		nodes[i].Name = strconv.Itoa(i)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		t := data.NewTreeJSONBuilder()
		for i := range nodes {
			rtest.OK(b, t.AddNode(&nodes[i]))
		}
		_, err := t.Finalize()
		rtest.OK(b, err)
	}
}

func TestLoadTree(t *testing.T) {
	repository.TestAllVersions(t, testLoadTree)
}

func testLoadTree(t *testing.T, version uint) {
	if rtest.BenchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping")
	}

	// archive a few files
	repo, _, _ := repository.TestRepositoryWithVersion(t, version)
	sn := archiver.TestSnapshot(t, repo, rtest.BenchArchiveDirectory, nil)

	nodes, err := data.LoadTree(context.TODO(), repo, *sn.Tree)
	rtest.OK(t, err)
	for item := range nodes {
		rtest.OK(t, item.Error)
	}
}

func TestTreeIteratorUnknownKeys(t *testing.T) {
	tests := []struct {
		name      string
		jsonData  string
		wantNodes []string
	}{
		{
			name:      "unknown key before nodes",
			jsonData:  `{"extra": "value", "nodes": [{"name": "test1"}, {"name": "test2"}]}`,
			wantNodes: []string{"test1", "test2"},
		},
		{
			name:      "unknown key after nodes",
			jsonData:  `{"nodes": [{"name": "test1"}, {"name": "test2"}], "extra": "value"}`,
			wantNodes: []string{"test1", "test2"},
		},
		{
			name:      "multiple unknown keys before nodes",
			jsonData:  `{"key1": "value1", "key2": 42, "nodes": [{"name": "test1"}]}`,
			wantNodes: []string{"test1"},
		},
		{
			name:      "multiple unknown keys after nodes",
			jsonData:  `{"nodes": [{"name": "test1"}], "key1": "value1", "key2": 42}`,
			wantNodes: []string{"test1"},
		},
		{
			name:      "unknown keys before and after nodes",
			jsonData:  `{"before": "value", "nodes": [{"name": "test1"}], "after": "value"}`,
			wantNodes: []string{"test1"},
		},
		{
			name:      "nested object as unknown value",
			jsonData:  `{"extra": {"nested": "value"}, "nodes": [{"name": "test1"}]}`,
			wantNodes: []string{"test1"},
		},
		{
			name:      "nested array as unknown value",
			jsonData:  `{"extra": [1, 2, 3], "nodes": [{"name": "test1"}]}`,
			wantNodes: []string{"test1"},
		},
		{
			name:      "complex nested structure as unknown value",
			jsonData:  `{"extra": {"obj": {"arr": [1, {"nested": true}]}}, "nodes": [{"name": "test1"}]}`,
			wantNodes: []string{"test1"},
		},
		{
			name:      "empty nodes array with unknown keys",
			jsonData:  `{"extra": "value", "nodes": []}`,
			wantNodes: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			it, err := data.NewTreeNodeIterator(strings.NewReader(tt.jsonData + "\n"))
			rtest.OK(t, err)

			var gotNodes []string
			for item := range it {
				rtest.OK(t, item.Error)
				gotNodes = append(gotNodes, item.Node.Name)
			}

			rtest.Equals(t, tt.wantNodes, gotNodes, "nodes mismatch")
		})
	}
}

func BenchmarkLoadTree(t *testing.B) {
	repository.BenchmarkAllVersions(t, benchmarkLoadTree)
}

func benchmarkLoadTree(t *testing.B, version uint) {
	if rtest.BenchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping")
	}

	// archive a few files
	repo, _, _ := repository.TestRepositoryWithVersion(t, version)
	sn := archiver.TestSnapshot(t, repo, rtest.BenchArchiveDirectory, nil)

	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		_, err := data.LoadTree(context.TODO(), repo, *sn.Tree)
		rtest.OK(t, err)
	}
}

func TestTreeFinderNilIterator(t *testing.T) {
	finder := data.NewTreeFinder(nil)
	defer finder.Close()
	node, err := finder.Find("foo")
	rtest.OK(t, err)
	rtest.Equals(t, node, nil, "finder should return nil node")
}

func TestTreeFinderError(t *testing.T) {
	testErr := errors.New("error")
	finder := data.NewTreeFinder(slices.Values([]data.NodeOrError{
		{Node: &data.Node{Name: "a"}, Error: nil},
		{Node: &data.Node{Name: "b"}, Error: nil},
		{Node: nil, Error: testErr},
	}))
	defer finder.Close()
	node, err := finder.Find("b")
	rtest.OK(t, err)
	rtest.Equals(t, node.Name, "b", "finder should return node with name b")

	node, err = finder.Find("c")
	rtest.Equals(t, err, testErr, "finder should return correcterror")
	rtest.Equals(t, node, nil, "finder should return nil node")
}

func TestTreeFinderNotFound(t *testing.T) {
	finder := data.NewTreeFinder(slices.Values([]data.NodeOrError{
		{Node: &data.Node{Name: "a"}, Error: nil},
	}))
	defer finder.Close()
	node, err := finder.Find("b")
	rtest.OK(t, err)
	rtest.Equals(t, node, nil, "finder should return nil node")
	// must also be ok multiple times
	node, err = finder.Find("c")
	rtest.OK(t, err)
	rtest.Equals(t, node, nil, "finder should return nil node")
}

func TestTreeFinderWrongOrder(t *testing.T) {
	finder := data.NewTreeFinder(slices.Values([]data.NodeOrError{
		{Node: &data.Node{Name: "d"}, Error: nil},
	}))
	defer finder.Close()
	node, err := finder.Find("b")
	rtest.OK(t, err)
	rtest.Equals(t, node, nil, "finder should return nil node")
	node, err = finder.Find("a")
	rtest.Assert(t, strings.Contains(err.Error(), "is not greater than"), "unexpected error: %v", err)
	rtest.Equals(t, node, nil, "finder should return nil node")
}

func TestFindTreeDirectory(t *testing.T) {
	repo := repository.TestRepository(t)
	sn := data.TestCreateSnapshot(t, repo, parseTimeUTC("2017-07-07 07:07:08"), 3)

	for _, exp := range []struct {
		subfolder string
		id        restic.ID
		err       error
	}{
		{"", restic.TestParseID("8804a5505fc3012e7d08b2843e9bda1bf3dc7644f64b542470340e1b4059f09f"), nil},
		{"/", restic.TestParseID("8804a5505fc3012e7d08b2843e9bda1bf3dc7644f64b542470340e1b4059f09f"), nil},
		{".", restic.TestParseID("8804a5505fc3012e7d08b2843e9bda1bf3dc7644f64b542470340e1b4059f09f"), nil},
		{"..", restic.ID{}, errors.New("path ..: not found")},
		{"file-1", restic.ID{}, errors.New("path file-1: not a directory")},
		{"dir-7", restic.TestParseID("1af51eb70cd4457d51db40d649bb75446a3eaa29b265916d411bb7ae971d4849"), nil},
		{"/dir-7", restic.TestParseID("1af51eb70cd4457d51db40d649bb75446a3eaa29b265916d411bb7ae971d4849"), nil},
		{"dir-7/", restic.TestParseID("1af51eb70cd4457d51db40d649bb75446a3eaa29b265916d411bb7ae971d4849"), nil},
		{"dir-7/dir-5", restic.TestParseID("f05534d2673964de698860e5069da1ee3c198acf21c187975c6feb49feb8e9c9"), nil},
	} {
		t.Run("", func(t *testing.T) {
			id, err := data.FindTreeDirectory(context.TODO(), repo, sn.Tree, exp.subfolder)
			if exp.err == nil {
				rtest.OK(t, err)
				rtest.Assert(t, exp.id == *id, "unexpected id, expected %v, got %v", exp.id, id)
			} else {
				rtest.Assert(t, exp.err.Error() == err.Error(), "unexpected err, expected %v, got %v", exp.err, err)
			}
		})
	}

	_, err := data.FindTreeDirectory(context.TODO(), repo, nil, "")
	rtest.Assert(t, err != nil, "missing error on null tree id")
}

func TestDualTreeIterator(t *testing.T) {
	testErr := errors.New("test error")

	tests := []struct {
		name     string
		tree1    []data.NodeOrError
		tree2    []data.NodeOrError
		expected []data.DualTree
	}{
		{
			name:     "both empty",
			tree1:    []data.NodeOrError{},
			tree2:    []data.NodeOrError{},
			expected: []data.DualTree{},
		},
		{
			name:  "tree1 empty",
			tree1: []data.NodeOrError{},
			tree2: []data.NodeOrError{
				{Node: &data.Node{Name: "a"}},
				{Node: &data.Node{Name: "b"}},
			},
			expected: []data.DualTree{
				{Tree1: nil, Tree2: &data.Node{Name: "a"}, Error: nil},
				{Tree1: nil, Tree2: &data.Node{Name: "b"}, Error: nil},
			},
		},
		{
			name: "tree2 empty",
			tree1: []data.NodeOrError{
				{Node: &data.Node{Name: "a"}},
				{Node: &data.Node{Name: "b"}},
			},
			tree2: []data.NodeOrError{},
			expected: []data.DualTree{
				{Tree1: &data.Node{Name: "a"}, Tree2: nil, Error: nil},
				{Tree1: &data.Node{Name: "b"}, Tree2: nil, Error: nil},
			},
		},
		{
			name: "identical trees",
			tree1: []data.NodeOrError{
				{Node: &data.Node{Name: "a"}},
				{Node: &data.Node{Name: "b"}},
			},
			tree2: []data.NodeOrError{
				{Node: &data.Node{Name: "a"}},
				{Node: &data.Node{Name: "b"}},
			},
			expected: []data.DualTree{
				{Tree1: &data.Node{Name: "a"}, Tree2: &data.Node{Name: "a"}, Error: nil},
				{Tree1: &data.Node{Name: "b"}, Tree2: &data.Node{Name: "b"}, Error: nil},
			},
		},
		{
			name: "disjoint trees",
			tree1: []data.NodeOrError{
				{Node: &data.Node{Name: "a"}},
				{Node: &data.Node{Name: "c"}},
			},
			tree2: []data.NodeOrError{
				{Node: &data.Node{Name: "b"}},
				{Node: &data.Node{Name: "d"}},
			},
			expected: []data.DualTree{
				{Tree1: &data.Node{Name: "a"}, Tree2: nil, Error: nil},
				{Tree1: nil, Tree2: &data.Node{Name: "b"}, Error: nil},
				{Tree1: &data.Node{Name: "c"}, Tree2: nil, Error: nil},
				{Tree1: nil, Tree2: &data.Node{Name: "d"}, Error: nil},
			},
		},
		{
			name: "overlapping trees",
			tree1: []data.NodeOrError{
				{Node: &data.Node{Name: "a"}},
				{Node: &data.Node{Name: "b"}},
				{Node: &data.Node{Name: "d"}},
			},
			tree2: []data.NodeOrError{
				{Node: &data.Node{Name: "b"}},
				{Node: &data.Node{Name: "c"}},
				{Node: &data.Node{Name: "d"}},
			},
			expected: []data.DualTree{
				{Tree1: &data.Node{Name: "a"}, Tree2: nil, Error: nil},
				{Tree1: &data.Node{Name: "b"}, Tree2: &data.Node{Name: "b"}, Error: nil},
				{Tree1: nil, Tree2: &data.Node{Name: "c"}, Error: nil},
				{Tree1: &data.Node{Name: "d"}, Tree2: &data.Node{Name: "d"}, Error: nil},
			},
		},
		{
			name: "error in tree1 during iteration",
			tree1: []data.NodeOrError{
				{Node: &data.Node{Name: "a"}},
				{Error: testErr},
			},
			tree2: []data.NodeOrError{
				{Node: &data.Node{Name: "c"}},
			},
			expected: []data.DualTree{
				{Tree1: nil, Tree2: nil, Error: testErr},
			},
		},
		{
			name: "error in tree2 during iteration",
			tree1: []data.NodeOrError{
				{Node: &data.Node{Name: "a"}},
			},
			tree2: []data.NodeOrError{
				{Node: &data.Node{Name: "b"}},
				{Error: testErr},
			},
			expected: []data.DualTree{
				{Tree1: &data.Node{Name: "a"}, Tree2: nil, Error: nil},
				{Tree1: nil, Tree2: nil, Error: testErr},
			},
		},
		{
			name:  "error at start of tree1",
			tree1: []data.NodeOrError{{Error: testErr}},
			tree2: []data.NodeOrError{{Node: &data.Node{Name: "b"}}},
			expected: []data.DualTree{
				{Tree1: nil, Tree2: nil, Error: testErr},
			},
		},
		{
			name:  "error at start of tree2",
			tree1: []data.NodeOrError{{Node: &data.Node{Name: "a"}}},
			tree2: []data.NodeOrError{{Error: testErr}},
			expected: []data.DualTree{
				{Tree1: nil, Tree2: nil, Error: testErr},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iter1 := slices.Values(tt.tree1)
			iter2 := slices.Values(tt.tree2)

			dualIter := data.DualTreeIterator(iter1, iter2)
			var results []data.DualTree
			for dt := range dualIter {
				results = append(results, dt)
			}

			rtest.Equals(t, len(tt.expected), len(results), "unexpected number of results")
			for i, exp := range tt.expected {
				rtest.Equals(t, exp.Error, results[i].Error, fmt.Sprintf("error mismatch at index %d", i))
				rtest.Equals(t, exp.Tree1, results[i].Tree1, fmt.Sprintf("Tree1 mismatch at index %d", i))
				rtest.Equals(t, exp.Tree2, results[i].Tree2, fmt.Sprintf("Tree2 mismatch at index %d", i))
			}
		})
	}

	t.Run("single use restriction", func(t *testing.T) {
		iter1 := slices.Values([]data.NodeOrError{{Node: &data.Node{Name: "a"}}})
		iter2 := slices.Values([]data.NodeOrError{{Node: &data.Node{Name: "b"}}})
		dualIter := data.DualTreeIterator(iter1, iter2)

		// First use should work
		var count int
		for range dualIter {
			count++
		}
		rtest.Assert(t, count > 0, "first iteration should produce results")

		// Second use should panic
		func() {
			defer func() {
				if r := recover(); r == nil {
					t.Fatal("expected panic on second use")
				}
			}()
			count = 0
			for range dualIter {
				// Should panic before reaching here
				count++
			}
			rtest.Equals(t, count, 0, "expected count to be 0")
		}()
	})
}
