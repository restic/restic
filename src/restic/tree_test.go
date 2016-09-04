package restic_test

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"restic"
	"restic/repository"
	. "restic/test"
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
	tempdir, err := ioutil.TempDir(TestTempDir, "restic-test-")
	OK(t, err)

	for _, test := range testFiles {
		file := filepath.Join(tempdir, test.name)
		dir := filepath.Dir(file)
		if dir != "." {
			OK(t, os.MkdirAll(dir, 0755))
		}

		f, err := os.Create(file)
		defer func() {
			OK(t, f.Close())
		}()

		OK(t, err)

		_, err = f.Write(test.content)
		OK(t, err)
	}

	return tempdir
}

func TestTree(t *testing.T) {
	dir := createTempDir(t)
	defer func() {
		if TestCleanupTempDirs {
			RemoveAll(t, dir)
		}
	}()
}

var testNodes = []restic.Node{
	restic.Node{Name: "normal"},
	restic.Node{Name: "with backslashes \\zzz"},
	restic.Node{Name: "test utf-8 föbärß"},
	restic.Node{Name: "test invalid \x00\x01\x02\x03\x04"},
	restic.Node{Name: "test latin1 \x75\x6d\x6c\xe4\xfc\x74\xf6\x6e\xdf\x6e\x6c\x6c"},
}

func TestNodeMarshal(t *testing.T) {
	for i, n := range testNodes {
		data, err := json.Marshal(&n)
		OK(t, err)

		var node restic.Node
		err = json.Unmarshal(data, &node)
		OK(t, err)

		if n.Name != node.Name {
			t.Fatalf("Node %d: Names are not equal, want: %q got: %q", i, n.Name, node.Name)
		}
	}
}

func TestNodeComparison(t *testing.T) {
	fi, err := os.Lstat("tree_test.go")
	OK(t, err)

	node, err := restic.NodeFromFileInfo("foo", fi)
	OK(t, err)

	n2 := *node
	Assert(t, node.Equals(n2), "nodes aren't equal")

	n2.Size--
	Assert(t, !node.Equals(n2), "nodes are equal")
}

func TestLoadTree(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	// save tree
	tree := restic.NewTree()
	id, err := repo.SaveTree(tree)
	OK(t, err)

	// save packs
	OK(t, repo.Flush())

	// load tree again
	tree2, err := repo.LoadTree(id)
	OK(t, err)

	Assert(t, tree.Equals(tree2),
		"trees are not equal: want %v, got %v",
		tree, tree2)
}
