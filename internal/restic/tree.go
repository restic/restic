package restic

import (
	"fmt"
	"sort"

	"github.com/restic/restic/internal/errors"

	"github.com/restic/restic/internal/debug"
)

// Tree is an ordered list of nodes.
type Tree struct {
	Nodes []*Node `json:"nodes"`
}

// NewTree creates a new tree object.
func NewTree() *Tree {
	return &Tree{
		Nodes: []*Node{},
	}
}

func (t *Tree) String() string {
	return fmt.Sprintf("Tree<%d nodes>", len(t.Nodes))
}

// Equals returns true if t and other have exactly the same nodes.
func (t *Tree) Equals(other *Tree) bool {
	if len(t.Nodes) != len(other.Nodes) {
		debug.Log("tree.Equals(): trees have different number of nodes")
		return false
	}

	for i := 0; i < len(t.Nodes); i++ {
		if !t.Nodes[i].Equals(*other.Nodes[i]) {
			debug.Log("tree.Equals(): node %d is different:", i)
			debug.Log("  %#v", t.Nodes[i])
			debug.Log("  %#v", other.Nodes[i])
			return false
		}
	}

	return true
}

// Insert adds a new node at the correct place in the tree.
func (t *Tree) Insert(node *Node) error {
	pos, found := t.find(node.Name)
	if found != nil {
		return errors.Errorf("node %q already present", node.Name)
	}

	// https://code.google.com/p/go-wiki/wiki/SliceTricks
	t.Nodes = append(t.Nodes, &Node{})
	copy(t.Nodes[pos+1:], t.Nodes[pos:])
	t.Nodes[pos] = node

	return nil
}

func (t *Tree) find(name string) (int, *Node) {
	pos := sort.Search(len(t.Nodes), func(i int) bool {
		return t.Nodes[i].Name >= name
	})

	if pos < len(t.Nodes) && t.Nodes[pos].Name == name {
		return pos, t.Nodes[pos]
	}

	return pos, nil
}

// Find returns a node with the given name, or nil if none could be found.
func (t *Tree) Find(name string) *Node {
	if t == nil {
		return nil
	}

	_, node := t.find(name)
	return node
}

// Sort sorts the nodes by name.
func (t *Tree) Sort() {
	list := Nodes(t.Nodes)
	sort.Sort(list)
	t.Nodes = list
}

// Subtrees returns a slice of all subtree IDs of the tree.
func (t *Tree) Subtrees() (trees IDs) {
	for _, node := range t.Nodes {
		if node.Type == "dir" && node.Subtree != nil {
			trees = append(trees, *node.Subtree)
		}
	}

	return trees
}
