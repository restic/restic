package restic_test

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

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
	tempdir, err := ioutil.TempDir(rtest.TestTempDir, "restic-test-")
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

func TestLoadTree(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	// save tree
	tree := restic.NewTree()
	id, err := repo.SaveTree(context.TODO(), tree)
	rtest.OK(t, err)

	// save packs
	rtest.OK(t, repo.Flush(context.Background()))

	// load tree again
	tree2, err := repo.LoadTree(context.TODO(), id)
	rtest.OK(t, err)

	rtest.Assert(t, tree.Equals(tree2),
		"trees are not equal: want %v, got %v",
		tree, tree2)
}
