package restic_test

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/restic/restic"
	"github.com/restic/restic/pack"
	. "github.com/restic/restic/test"
)

var testFiles = []struct {
	name    string
	content []byte
}{
	{"foo", []byte("bar")},
	{"bar/foo2", []byte("bar2")},
	{"bar/bla/blubb", []byte("This is just a test!\n")},
}

// prepareDir creates a temporary directory and returns it.
func prepareDir(t *testing.T) string {
	tempdir, err := ioutil.TempDir(*TestTempDir, "restic-test-")
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
	dir := prepareDir(t)
	defer func() {
		if *TestCleanup {
			OK(t, os.RemoveAll(dir))
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

	n2.Size -= 1
	Assert(t, !node.Equals(n2), "nodes are equal")
}

func TestLoadTree(t *testing.T) {
	server := SetupBackend(t)
	defer TeardownBackend(t, server)
	key := SetupKey(t, server, "geheim")
	server.SetKey(key)

	// save tree
	tree := restic.NewTree()
	id, err := server.SaveJSON(pack.Tree, tree)
	OK(t, err)

	// save packs
	OK(t, server.Flush())

	// load tree again
	tree2, err := restic.LoadTree(server, id)
	OK(t, err)

	Assert(t, tree.Equals(tree2),
		"trees are not equal: want %v, got %v",
		tree, tree2)
}
