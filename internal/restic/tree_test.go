package restic_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/restic/restic/internal/archiver"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"golang.org/x/sync/errgroup"
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

var testNodes = []restic.Node{
	{Name: "normal"},
	{Name: "with backslashes \\zzz"},
	{Name: "test utf-8 föbärß"},
	{Name: "test invalid \x00\x01\x02\x03\x04"},
	{Name: "test latin1 \x75\x6d\x6c\xe4\xfc\x74\xf6\x6e\xdf\x6e\x6c\x6c"},
}

func TestNodeMarshal(t *testing.T) {
	for i, n := range testNodes {
		data, err := json.Marshal(&n)
		rtest.OK(t, err)

		var node restic.Node
		err = json.Unmarshal(data, &node)
		rtest.OK(t, err)

		if n.Name != node.Name {
			t.Fatalf("Node %d: Names are not equal, want: %q got: %q", i, n.Name, node.Name)
		}
	}
}

func TestNodeComparison(t *testing.T) {
	fi, err := os.Lstat("tree_test.go")
	rtest.OK(t, err)

	node, err := restic.NodeFromFileInfo("tree_test.go", fi)
	rtest.OK(t, err)

	n2 := *node
	rtest.Assert(t, node.Equals(n2), "nodes aren't equal")

	n2.Size--
	rtest.Assert(t, !node.Equals(n2), "nodes are equal")
}

func TestEmptyLoadTree(t *testing.T) {
	repo := repository.TestRepository(t)

	var wg errgroup.Group
	repo.StartPackUploader(context.TODO(), &wg)
	// save tree
	tree := restic.NewTree(0)
	id, err := restic.SaveTree(context.TODO(), repo, tree)
	rtest.OK(t, err)

	// save packs
	rtest.OK(t, repo.Flush(context.Background()))

	// load tree again
	tree2, err := restic.LoadTree(context.TODO(), repo, id)
	rtest.OK(t, err)

	rtest.Assert(t, tree.Equals(tree2),
		"trees are not equal: want %v, got %v",
		tree, tree2)
}

func TestTreeEqualSerialization(t *testing.T) {
	files := []string{"node.go", "tree.go", "tree_test.go"}
	for i := 1; i <= len(files); i++ {
		tree := restic.NewTree(i)
		builder := restic.NewTreeJSONBuilder()

		for _, fn := range files[:i] {
			fi, err := os.Lstat(fn)
			rtest.OK(t, err)
			node, err := restic.NodeFromFileInfo(fn, fi)
			rtest.OK(t, err)

			rtest.OK(t, tree.Insert(node))
			rtest.OK(t, builder.AddNode(node))

			rtest.Assert(t, tree.Insert(node) != nil, "no error on duplicate node")
			rtest.Assert(t, builder.AddNode(node) != nil, "no error on duplicate node")
			rtest.Assert(t, errors.Is(builder.AddNode(node), restic.ErrTreeNotOrdered), "wrong error returned")
		}

		treeBytes, err := json.Marshal(tree)
		treeBytes = append(treeBytes, '\n')
		rtest.OK(t, err)

		stiBytes, err := builder.Finalize()
		rtest.OK(t, err)

		// compare serialization of an individual node and the SaveTreeIterator
		rtest.Equals(t, treeBytes, stiBytes)
	}
}

func BenchmarkBuildTree(b *testing.B) {
	const size = 100 // Directories of this size are not uncommon.

	nodes := make([]restic.Node, size)
	for i := range nodes {
		// Archiver.SaveTree inputs in sorted order, so do that here too.
		nodes[i].Name = strconv.Itoa(i)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		t := restic.NewTree(size)

		for i := range nodes {
			_ = t.Insert(&nodes[i])
		}
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
	repo := repository.TestRepositoryWithVersion(t, version)
	sn := archiver.TestSnapshot(t, repo, rtest.BenchArchiveDirectory, nil)
	rtest.OK(t, repo.Flush(context.Background()))

	_, err := restic.LoadTree(context.TODO(), repo, *sn.Tree)
	rtest.OK(t, err)
}

func BenchmarkLoadTree(t *testing.B) {
	repository.BenchmarkAllVersions(t, benchmarkLoadTree)
}

func benchmarkLoadTree(t *testing.B, version uint) {
	if rtest.BenchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping")
	}

	// archive a few files
	repo := repository.TestRepositoryWithVersion(t, version)
	sn := archiver.TestSnapshot(t, repo, rtest.BenchArchiveDirectory, nil)
	rtest.OK(t, repo.Flush(context.Background()))

	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		_, err := restic.LoadTree(context.TODO(), repo, *sn.Tree)
		rtest.OK(t, err)
	}
}
