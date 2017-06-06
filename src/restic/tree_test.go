package restic_test

import (
	"encoding/json"
	"os"
	"testing"

	"restic"
	"restic/repository"
	. "restic/test"
)

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

	node, err := restic.NodeFromFileInfo("tree_test.go", fi)
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
