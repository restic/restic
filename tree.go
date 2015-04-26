package restic

import (
	"errors"
	"fmt"
	"sort"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/debug"
	"github.com/restic/restic/server"
)

type Tree struct {
	Nodes []*Node `json:"nodes"`
	Map   *Map    `json:"map"`
}

var (
	ErrNodeNotFound      = errors.New("named node not found")
	ErrNodeAlreadyInTree = errors.New("node already present")
)

func NewTree() *Tree {
	return &Tree{
		Nodes: []*Node{},
		Map:   NewMap(),
	}
}

func (t Tree) String() string {
	return fmt.Sprintf("Tree<%d nodes, %d blobs>", len(t.Nodes), len(t.Map.list))
}

func LoadTree(s *server.Server, blob server.Blob) (*Tree, error) {
	tree := &Tree{}
	err := s.LoadJSON(backend.Tree, blob, tree)
	if err != nil {
		return nil, err
	}

	return tree, nil
}

// Equals returns true if t and other have exactly the same nodes and map.
func (t Tree) Equals(other Tree) bool {
	if len(t.Nodes) != len(other.Nodes) {
		debug.Log("Tree.Equals", "tree.Equals(): trees have different number of nodes")
		return false
	}

	if !t.Map.Equals(other.Map) {
		debug.Log("Tree.Equals", "tree.Equals(): maps aren't equal")
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
	pos, _, err := t.find(node.Name)
	if err == nil {
		// already present
		return ErrNodeAlreadyInTree
	}

	// https://code.google.com/p/go-wiki/wiki/SliceTricks
	t.Nodes = append(t.Nodes, &Node{})
	copy(t.Nodes[pos+1:], t.Nodes[pos:])
	t.Nodes[pos] = node

	return nil
}

func (t Tree) find(name string) (int, *Node, error) {
	pos := sort.Search(len(t.Nodes), func(i int) bool {
		return t.Nodes[i].Name >= name
	})

	if pos < len(t.Nodes) && t.Nodes[pos].Name == name {
		return pos, t.Nodes[pos], nil
	}

	return pos, nil, ErrNodeNotFound
}

func (t Tree) Find(name string) (*Node, error) {
	_, node, err := t.find(name)
	return node, err
}
