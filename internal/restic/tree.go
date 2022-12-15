package restic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/restic/restic/internal/errors"

	"github.com/restic/restic/internal/debug"
)

// Tree is an ordered list of nodes.
type Tree struct {
	Nodes []*Node `json:"nodes"`
}

// NewTree creates a new tree object with the given initial capacity.
func NewTree(capacity int) *Tree {
	return &Tree{
		Nodes: make([]*Node, 0, capacity),
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

	// https://github.com/golang/go/wiki/SliceTricks
	t.Nodes = append(t.Nodes, nil)
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

type BlobLoader interface {
	LoadBlob(context.Context, BlobType, ID, []byte) ([]byte, error)
}

// LoadTree loads a tree from the repository.
func LoadTree(ctx context.Context, r BlobLoader, id ID) (*Tree, error) {
	debug.Log("load tree %v", id)

	buf, err := r.LoadBlob(ctx, TreeBlob, id, nil)
	if err != nil {
		return nil, err
	}

	t := &Tree{}
	err = json.Unmarshal(buf, t)
	if err != nil {
		return nil, err
	}

	return t, nil
}

type BlobSaver interface {
	SaveBlob(context.Context, BlobType, []byte, ID, bool) (ID, bool, int, error)
}

// SaveTree stores a tree into the repository and returns the ID. The ID is
// checked against the index. The tree is only stored when the index does not
// contain the ID.
func SaveTree(ctx context.Context, r BlobSaver, t *Tree) (ID, error) {
	buf, err := json.Marshal(t)
	if err != nil {
		return ID{}, errors.Wrap(err, "MarshalJSON")
	}

	// append a newline so that the data is always consistent (json.Encoder
	// adds a newline after each object)
	buf = append(buf, '\n')

	id, _, _, err := r.SaveBlob(ctx, TreeBlob, buf, ID{}, false)
	return id, err
}

var ErrTreeNotOrdered = errors.New("nodes are not ordered or duplicate")

type TreeJSONBuilder struct {
	buf      bytes.Buffer
	lastName string
}

func NewTreeJSONBuilder() *TreeJSONBuilder {
	tb := &TreeJSONBuilder{}
	_, _ = tb.buf.WriteString(`{"nodes":[`)
	return tb
}

func (builder *TreeJSONBuilder) AddNode(node *Node) error {
	if node.Name <= builder.lastName {
		return fmt.Errorf("node %q, last%q: %w", node.Name, builder.lastName, ErrTreeNotOrdered)
	}
	if builder.lastName != "" {
		_ = builder.buf.WriteByte(',')
	}
	builder.lastName = node.Name

	val, err := json.Marshal(node)
	if err != nil {
		return err
	}
	_, _ = builder.buf.Write(val)
	return nil
}

func (builder *TreeJSONBuilder) Finalize() ([]byte, error) {
	// append a newline so that the data is always consistent (json.Encoder
	// adds a newline after each object)
	_, _ = builder.buf.WriteString("]}\n")
	buf := builder.buf.Bytes()
	// drop reference to buffer
	builder.buf = bytes.Buffer{}
	return buf, nil
}
