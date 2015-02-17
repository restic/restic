package restic

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"sync"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/debug"
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

func LoadTree(s Server, id backend.ID) (*Tree, error) {
	tree := &Tree{}
	err := s.LoadJSONID(backend.Tree, id, tree)
	if err != nil {
		return nil, err
	}

	return tree, nil
}

// LoadTreeRecursive loads the tree and all subtrees via s.
func LoadTreeRecursive(path string, s Server, blob Blob) (*Tree, error) {
	// TODO: load subtrees in parallel
	tree, err := LoadTree(s, blob.Storage)
	if err != nil {
		return nil, err
	}

	for _, n := range tree.Nodes {
		n.path = filepath.Join(path, n.Name)
		if n.Type == "dir" && n.Subtree != nil {
			subtreeBlob, err := tree.Map.FindID(n.Subtree)
			if err != nil {
				return nil, err
			}

			t, err := LoadTreeRecursive(n.path, s, subtreeBlob)
			if err != nil {
				return nil, err
			}

			n.tree = t
		}
	}

	return tree, nil
}

// CopyFrom recursively copies all content from other to t.
func (t Tree) CopyFrom(other *Tree, s *Server) error {
	debug.Log("Tree.CopyFrom", "CopyFrom(%v)\n", other)
	for _, node := range t.Nodes {
		// only process files and dirs
		if node.Type != "file" && node.Type != "dir" {
			continue
		}

		// find entry in other tree
		oldNode, err := other.Find(node.Name)

		// if the node could not be found or the type has changed, proceed to the next
		if err == ErrNodeNotFound || node.Type != oldNode.Type {
			debug.Log("Tree.CopyFrom", "  node %v is new\n", node)
			continue
		}

		if node.Type == "file" {
			// compare content
			if node.SameContent(oldNode) {
				debug.Log("Tree.CopyFrom", "  file node %v has same content\n", node)

				// check if all content is still available in the repository
				for _, id := range oldNode.Content {
					blob, err := other.Map.FindID(id)
					if err != nil {
						continue
					}

					if ok, err := s.Test(backend.Data, blob.Storage); !ok || err != nil {
						continue
					}
				}

				// copy Content
				node.Content = oldNode.Content

				// copy storage IDs
				for _, id := range node.Content {
					blob, err := other.Map.FindID(id)
					if err != nil {
						return err
					}

					debug.Log("Tree.CopyFrom", "    insert blob %v\n", blob)
					t.Map.Insert(blob)
				}
			}
		} else if node.Type == "dir" {
			// fill in all subtrees from old subtree
			err := node.tree.CopyFrom(oldNode.tree, s)
			if err != nil {
				return err
			}

			// check if tree has changed
			if node.tree.Equals(*oldNode.tree) {
				debug.Log("Tree.CopyFrom", "  tree node %v has same content\n", node)

				// if nothing has changed, copy subtree ID
				node.Subtree = oldNode.Subtree

				// and store blob in bloblist
				blob, err := other.Map.FindID(oldNode.Subtree)
				if err != nil {
					return err
				}

				debug.Log("Tree.CopyFrom", "    insert blob %v\n", blob)
				t.Map.Insert(blob)
			} else {
				debug.Log("Tree.CopyFrom", "  trees are not equal: %v\n", node)
				debug.Log("Tree.CopyFrom", "    %#v\n", node.tree)
				debug.Log("Tree.CopyFrom", "    %#v\n", oldNode.tree)
			}
		}
	}

	return nil
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

func (t Tree) Stat() Stat {
	s := Stat{}
	for _, n := range t.Nodes {
		switch n.Type {
		case "file":
			s.Files++
			s.Bytes += n.Size
		case "dir":
			s.Dirs++
			if n.tree != nil {
				s.Add(n.tree.Stat())
			}
		}
	}

	return s
}

func (t Tree) StatTodo() Stat {
	s := Stat{}
	for _, n := range t.Nodes {
		switch n.Type {
		case "file":
			if n.Content == nil {
				s.Files++
				s.Bytes += n.Size
			}
		case "dir":
			if n.Subtree == nil {
				s.Dirs++
				if n.tree != nil {
					s.Add(n.tree.StatTodo())
				}
			}
		}
	}

	return s
}

// EachNode calls fn recursively for each node in t and all subtrees
// (depth-first). This in done concurrently in p goroutines.
func (t *Tree) EachNode(p int, fn func(*Node)) {
	processNodeWorker := func(wg *sync.WaitGroup, ch <-chan *Node, f func(*Node)) {
		for node := range ch {
			f(node)
		}
		wg.Done()
	}

	// start workers
	var wg sync.WaitGroup
	ch := make(chan *Node)

	for i := 0; i < p; i++ {
		wg.Add(1)
		go processNodeWorker(&wg, ch, fn)
	}

	var processTree func(t *Tree, ch chan<- *Node)
	processTree = func(t *Tree, ch chan<- *Node) {
		for _, n := range t.Nodes {
			if n.Type == "dir" && n.Tree() != nil {
				processTree(n.Tree(), ch)
			}

			ch <- n
		}
	}

	// run on root
	processTree(t, ch)
	close(ch)

	// wait for all goroutines to terminate
	wg.Wait()
}

// EachSubtree calls fn recursively for t and all subtrees (depth-first). This
// is done concurrently in p goroutines.
func (t *Tree) EachSubtree(p int, fn func(*Tree)) {
	processTreeWorker := func(wg *sync.WaitGroup, ch <-chan *Tree, f func(*Tree)) {
		for tree := range ch {
			f(tree)
		}
		wg.Done()
	}

	// start workers
	var wg sync.WaitGroup
	ch := make(chan *Tree)

	for i := 0; i < p; i++ {
		wg.Add(1)
		go processTreeWorker(&wg, ch, fn)
	}

	// extra variable needed for recursion
	var processTree func(t *Tree, ch chan<- *Tree)
	processTree = func(t *Tree, ch chan<- *Tree) {
		for _, n := range t.Nodes {
			if n.Type == "dir" && n.Tree() != nil {
				processTree(n.Tree(), ch)
			}
		}

		ch <- t
	}

	// run on root
	processTree(t, ch)
	close(ch)

	// wait for all goroutines to terminate
	wg.Wait()
}
