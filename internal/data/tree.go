package data

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"io"
	"iter"
	"path"
	"strings"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
)

// For documentation purposes only:
// // Tree is an ordered list of nodes.
// type Tree struct {
//         Nodes []*Node `json:"nodes"`
// }

var ErrTreeNotOrdered = errors.New("nodes are not ordered or duplicate")

type treeIterator struct {
	dec     json.Decoder
	started bool
}

type NodeOrError struct {
	Node  *Node
	Error error
}

type TreeNodeIterator = iter.Seq[NodeOrError]

func NewTreeNodeIterator(rd io.Reader) (TreeNodeIterator, error) {
	t := &treeIterator{
		dec: *json.NewDecoder(rd),
	}

	err := t.init()
	if err != nil {
		return nil, err
	}

	return func(yield func(NodeOrError) bool) {
		if t.started {
			panic("tree iterator is single use only")
		}
		t.started = true
		for {
			n, err := t.next()
			if err != nil && errors.Is(err, io.EOF) {
				return
			}
			if !yield(NodeOrError{Node: n, Error: err}) {
				return
			}
			// errors are final
			if err != nil {
				return
			}
		}
	}, nil
}

func (t *treeIterator) init() error {
	// A tree is expected to be encoded as a JSON object with a single key "nodes".
	// However, for future-proofness, we allow unknown keys before and after the "nodes" key.
	// The following is the expected format:
	// `{"nodes":[...]}`

	if err := t.assertToken(json.Delim('{')); err != nil {
		return err
	}
	// Skip unknown keys until we find "nodes"
	for {
		token, err := t.dec.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok {
			return errors.Errorf("error decoding tree: expected string key, got %v", token)
		}
		if key == "nodes" {
			// Found "nodes", proceed to read the array
			if err := t.assertToken(json.Delim('[')); err != nil {
				return err
			}
			return nil
		}
		// Unknown key, decode its value into RawMessage and discard it
		var raw json.RawMessage
		if err := t.dec.Decode(&raw); err != nil {
			return err
		}
	}
}

func (t *treeIterator) next() (*Node, error) {
	if t.dec.More() {
		var n Node
		err := t.dec.Decode(&n)
		if err != nil {
			return nil, err
		}
		return &n, nil
	}

	if err := t.assertToken(json.Delim(']')); err != nil {
		return nil, err
	}
	// Skip unknown keys after the array until we find the closing brace
	for {
		token, err := t.dec.Token()
		if err != nil {
			return nil, err
		}
		if token == json.Delim('}') {
			return nil, io.EOF
		}
		// We have an unknown key, decode its value into RawMessage and discard it
		var raw json.RawMessage
		if err := t.dec.Decode(&raw); err != nil {
			return nil, err
		}
	}
}

func (t *treeIterator) assertToken(token json.Token) error {
	to, err := t.dec.Token()
	if err != nil {
		return err
	}
	if to != token {
		return errors.Errorf("error decoding tree: expected %v, got %v", token, to)
	}
	return nil
}

func LoadTree(ctx context.Context, loader restic.BlobLoader, content restic.ID) (TreeNodeIterator, error) {
	rd, err := loader.LoadBlob(ctx, restic.TreeBlob, content, nil)
	if err != nil {
		return nil, err
	}
	return NewTreeNodeIterator(bytes.NewReader(rd))
}

type TreeFinder struct {
	next    func() (NodeOrError, bool)
	stop    func()
	current *Node
	last    string
}

func NewTreeFinder(tree TreeNodeIterator) *TreeFinder {
	if tree == nil {
		return &TreeFinder{stop: func() {}}
	}
	next, stop := iter.Pull(tree)
	return &TreeFinder{next: next, stop: stop}
}

// Find finds the node with the given name. If the node is not found, it returns nil.
// If Find was called before, the new name must be strictly greater than the last name.
func (t *TreeFinder) Find(name string) (*Node, error) {
	if t.next == nil {
		return nil, nil
	}
	if name <= t.last {
		return nil, errors.Errorf("name %q is not greater than last name %q", name, t.last)
	}
	t.last = name
	// loop until `t.current.Name` is >= name
	for t.current == nil || t.current.Name < name {
		current, ok := t.next()
		if current.Error != nil {
			return nil, current.Error
		}
		if !ok {
			return nil, nil
		}
		t.current = current.Node
	}

	if t.current.Name == name {
		// forget the current node to free memory as early as possible
		current := t.current
		t.current = nil
		return current, nil
	}
	// we have already passed the name
	return nil, nil
}

func (t *TreeFinder) Close() {
	t.stop()
}

type TreeWriter struct {
	builder *TreeJSONBuilder
	saver   restic.BlobSaver
}

func NewTreeWriter(saver restic.BlobSaver) *TreeWriter {
	builder := NewTreeJSONBuilder()
	return &TreeWriter{builder: builder, saver: saver}
}

func (t *TreeWriter) AddNode(node *Node) error {
	return t.builder.AddNode(node)
}

func (t *TreeWriter) Finalize(ctx context.Context) (restic.ID, error) {
	buf, err := t.builder.Finalize()
	if err != nil {
		return restic.ID{}, err
	}
	id, _, _, err := t.saver.SaveBlob(ctx, restic.TreeBlob, buf, restic.ID{}, false)
	return id, err
}

// Count returns the number of nodes in the tree
func (t *TreeWriter) Count() int {
	return t.builder.Count()
}

func SaveTree(ctx context.Context, saver restic.BlobSaver, nodes TreeNodeIterator) (restic.ID, error) {
	treeWriter := NewTreeWriter(saver)
	for item := range nodes {
		if item.Error != nil {
			return restic.ID{}, item.Error
		}
		err := treeWriter.AddNode(item.Node)
		if err != nil {
			return restic.ID{}, err
		}
	}
	return treeWriter.Finalize(ctx)
}

type TreeJSONBuilder struct {
	buf        bytes.Buffer
	lastName   string
	countNodes int
}

func NewTreeJSONBuilder() *TreeJSONBuilder {
	tb := &TreeJSONBuilder{}
	_, _ = tb.buf.WriteString(`{"nodes":[`)
	return tb
}

func (builder *TreeJSONBuilder) AddNode(node *Node) error {
	if node.Name <= builder.lastName {
		return fmt.Errorf("node %q, last %q: %w", node.Name, builder.lastName, ErrTreeNotOrdered)
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
	builder.countNodes++
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

// Count returns the number of nodes in the tree
func (builder *TreeJSONBuilder) Count() int {
	return builder.countNodes
}

func FindTreeDirectory(ctx context.Context, repo restic.BlobLoader, id *restic.ID, dir string) (*restic.ID, error) {
	if id == nil {
		return nil, errors.New("tree id is null")
	}

	dirs := strings.Split(path.Clean(dir), "/")
	subfolder := ""

	for _, name := range dirs {
		if name == "" || name == "." {
			continue
		}
		subfolder = path.Join(subfolder, name)
		tree, err := LoadTree(ctx, repo, *id)
		if err != nil {
			return nil, fmt.Errorf("path %s: %w", subfolder, err)
		}
		finder := NewTreeFinder(tree)
		node, err := finder.Find(name)
		finder.Close()
		if err != nil {
			return nil, fmt.Errorf("path %s: %w", subfolder, err)
		}
		if node == nil {
			return nil, fmt.Errorf("path %s: not found", subfolder)
		}
		if node.Type != NodeTypeDir || node.Subtree == nil {
			return nil, fmt.Errorf("path %s: not a directory", subfolder)
		}
		id = node.Subtree
	}
	return id, nil
}

type peekableNodeIterator struct {
	iter  func() (NodeOrError, bool)
	stop  func()
	value *Node
}

func newPeekableNodeIterator(tree TreeNodeIterator) (*peekableNodeIterator, error) {
	iter, stop := iter.Pull(tree)
	it := &peekableNodeIterator{iter: iter, stop: stop}
	err := it.Next()
	if err != nil {
		it.Close()
		return nil, err
	}
	return it, nil
}

func (i *peekableNodeIterator) Next() error {
	item, ok := i.iter()
	if item.Error != nil || !ok {
		i.value = nil
		return item.Error
	}
	i.value = item.Node
	return nil
}

func (i *peekableNodeIterator) Peek() *Node {
	return i.value
}

func (i *peekableNodeIterator) Close() {
	i.stop()
}

type DualTree struct {
	Tree1 *Node
	Tree2 *Node
	Error error
}

// DualTreeIterator iterates over two trees in parallel. It returns a sequence of DualTree structs.
// The sequence is terminated when both trees are exhausted. The error field must be checked before
// accessing any of the nodes.
func DualTreeIterator(tree1, tree2 TreeNodeIterator) iter.Seq[DualTree] {
	started := false
	return func(yield func(DualTree) bool) {
		if started {
			panic("tree iterator is single use only")
		}
		started = true
		iter1, err := newPeekableNodeIterator(tree1)
		if err != nil {
			yield(DualTree{Tree1: nil, Tree2: nil, Error: err})
			return
		}
		defer iter1.Close()
		iter2, err := newPeekableNodeIterator(tree2)
		if err != nil {
			yield(DualTree{Tree1: nil, Tree2: nil, Error: err})
			return
		}
		defer iter2.Close()

		for {
			node1 := iter1.Peek()
			node2 := iter2.Peek()
			if node1 == nil && node2 == nil {
				// both iterators are exhausted
				break
			} else if node1 != nil && node2 != nil {
				// if both nodes have a different name, only keep the first one
				if node1.Name < node2.Name {
					node2 = nil
				} else if node1.Name > node2.Name {
					node1 = nil
				}
			}

			// non-nil nodes will be processed in the following, so advance the corresponding iterator
			if node1 != nil {
				if err = iter1.Next(); err != nil {
					break
				}
			}
			if node2 != nil {
				if err = iter2.Next(); err != nil {
					break
				}
			}

			if !yield(DualTree{Tree1: node1, Tree2: node2, Error: err}) {
				return
			}
		}
		if err != nil {
			yield(DualTree{Tree1: nil, Tree2: nil, Error: err})
			return
		}
	}
}
