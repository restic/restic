package restic

import (
	"fmt"
	"sort"

	"github.com/pkg/errors"

	"restic/backend"
	"restic/debug"
	"restic/pack"
)

type Tree struct {
	Nodes []*Node `json:"nodes"`
}

var (
	ErrNodeNotFound      = errors.New("named node not found")
	ErrNodeAlreadyInTree = errors.New("node already present")
)

func NewTree() *Tree {
	return &Tree{
		Nodes: []*Node{},
	}
}

func (t Tree) String() string {
	return fmt.Sprintf("Tree<%d nodes>", len(t.Nodes))
}

type TreeLoader interface {
	LoadJSONPack(pack.BlobType, backend.ID, interface{}) error
}

func LoadTree(repo TreeLoader, id backend.ID) (*Tree, error) {
	tree := &Tree{}
	err := repo.LoadJSONPack(pack.Tree, id, tree)
	if err != nil {
		return nil, err
	}

	return tree, nil
}

// Equals returns true if t and other have exactly the same nodes.
func (t Tree) Equals(other *Tree) bool {
	if len(t.Nodes) != len(other.Nodes) {
		debug.Log("Tree.Equals", "tree.Equals(): trees have different number of nodes")
		return false
	}

	for i := 0; i < len(t.Nodes); i++ {
		if !t.Nodes[i].Equals(*other.Nodes[i]) {
			debug.Log("Tree.Equals", "tree.Equals(): node %d is different:", i)
			debug.Log("Tree.Equals", "  %#v", t.Nodes[i])
			debug.Log("Tree.Equals", "  %#v", other.Nodes[i])
			return false
		}
	}

	return true
}

func (t *Tree) Insert(node *Node) error {
	pos, _, err := t.binarySearch(node.Name)
	if err == nil {
		return ErrNodeAlreadyInTree
	}

	// https://code.google.com/p/go-wiki/wiki/SliceTricks
	t.Nodes = append(t.Nodes, &Node{})
	copy(t.Nodes[pos+1:], t.Nodes[pos:])
	t.Nodes[pos] = node

	return nil
}

func (t Tree) binarySearch(name string) (int, *Node, error) {
	pos := sort.Search(len(t.Nodes), func(i int) bool {
		return t.Nodes[i].Name >= name
	})

	if pos < len(t.Nodes) && t.Nodes[pos].Name == name {
		return pos, t.Nodes[pos], nil
	}

	return pos, nil, ErrNodeNotFound
}

func (t Tree) Find(name string) (*Node, error) {
	_, node, err := t.binarySearch(name)
	return node, err
}

// Subtrees returns a slice of all subtree IDs of the tree.
func (t Tree) Subtrees() (trees backend.IDs) {
	for _, node := range t.Nodes {
		if node.Type == "dir" && node.Subtree != nil {
			trees = append(trees, *node.Subtree)
		}
	}

	return trees
}
