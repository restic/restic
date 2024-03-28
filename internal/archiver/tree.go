package archiver

import (
	"fmt"
	"sort"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
)

// Tree recursively defines how a snapshot should look like when
// archived.
//
// When `FileMetadata` is set, this is a leaf node and the contents of `FileMetadata` should be
// inserted at this point in the tree.
//
// The attribute `Root` is used to distinguish between files/dirs which have
// the same name, but live in a separate directory on the local file system.
//
// `FileInfoPath` is used to extract metadata for intermediate (=non-leaf)
// trees.
type Tree struct {
	Nodes        map[string]Tree
	FileMetadata restic.LazyFileMetadata // where the files/dirs to be saved are found
	FileInfoPath restic.RootDirectory    // where the dir can be found that is not included itself, but its subdirs
	Root         restic.RootDirectory    // parent directory of the tree
}

// Add adds a new file or directory to the tree.
func (t *Tree) Add(element restic.LazyFileMetadata) error {
	if element == nil {
		panic("invalid element (nil value)")
	}

	if t.Nodes == nil {
		t.Nodes = make(map[string]Tree)
	}

	pc, virtualPrefix := element.PathComponents(false)
	if len(pc) == 0 {
		return errors.New("invalid path (no path components)")
	}

	name := pc[0]
	root := element.RootDirectory()
	tree := Tree{Root: root}

	origName := name
	i := 0
	for {
		other, ok := t.Nodes[name]
		if !ok {
			break
		}

		i++
		if other.Root.Equal(root) {
			tree = other
			break
		}

		// resolve conflict and try again
		name = fmt.Sprintf("%s-%d", origName, i)
		continue
	}

	if len(pc) > 1 {
		subroot := root.Join(origName)
		if virtualPrefix {
			// use the original root dir if this is a virtual directory (volume name on Windows)
			subroot = root
		}
		err := tree.add(element, subroot, pc[1:])
		if err != nil {
			return err
		}
		tree.FileInfoPath = subroot
	} else {
		tree.FileMetadata = element
	}

	t.Nodes[name] = tree
	return nil
}

// add adds a new target path into the tree.
func (t *Tree) add(target restic.LazyFileMetadata, root restic.RootDirectory, pc []string) error {
	if len(pc) == 0 {
		return errors.Errorf("invalid path %q", target)
	}

	if t.Nodes == nil {
		t.Nodes = make(map[string]Tree)
	}

	name := pc[0]

	if len(pc) == 1 {
		tree, ok := t.Nodes[name]

		if !ok {
			t.Nodes[name] = Tree{FileMetadata: target}
			return nil
		}

		if tree.FileMetadata != nil {
			return errors.Errorf("path is already set for target %v", target)
		}
		tree.FileMetadata = target
		t.Nodes[name] = tree
		return nil
	}

	tree := Tree{}
	if other, ok := t.Nodes[name]; ok {
		tree = other
	}

	subroot := root.Join(name)
	tree.FileInfoPath = subroot

	err := tree.add(target, subroot, pc[1:])
	if err != nil {
		return err
	}
	t.Nodes[name] = tree

	return nil
}

func (t Tree) String() string {
	return formatTree(t, "")
}

// Leaf returns true if this is a leaf node, which means FileMetadata is set to a
// non-nul value and the contents of FileMetadata should be inserted at this point
// in the tree.
func (t Tree) Leaf() bool {
	return t.FileMetadata != nil
}

// NodeNames returns the sorted list of subtree names.
func (t Tree) NodeNames() []string {
	// iterate over the nodes of atree in lexicographic (=deterministic) order
	names := make([]string, 0, len(t.Nodes))
	for name := range t.Nodes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// formatTree returns a text representation of the tree t.
func formatTree(t Tree, indent string) (s string) {
	for name, node := range t.Nodes {
		s += fmt.Sprintf("%v/%v, root %q, path %q, meta %q\n", indent, name, node.Root, node.FileMetadata, node.FileInfoPath)
		s += formatTree(node, indent+"    ")
	}
	return s
}

// unrollTree unrolls the tree so that only leaf nodes have Path set.
func unrollTree(t *Tree) error {
	// if the current tree is a leaf node (Path is set) and has additional
	// nodes, add the contents of Path to the nodes.
	if t.FileMetadata != nil && len(t.Nodes) > 0 {
		debug.Log("resolve path %v", t.FileMetadata.Path())
		entries, err := t.FileMetadata.ChildrenWithFlag(0)
		if err != nil {
			return err
		}

		for _, entry := range entries {
			if node, ok := t.Nodes[entry.Name()]; ok {
				if node.FileMetadata == nil {
					node.FileMetadata = entry
					t.Nodes[entry.Name()] = node
					continue
				}

				if node.FileMetadata.Equal(entry) {
					continue
				}

				return errors.Errorf("tree unrollTree: collision on path, node %#v, path %q", node, entry.Path())
			}
			t.Nodes[entry.Name()] = Tree{FileMetadata: entry}
		}
		t.FileMetadata = nil
	}

	for i, subtree := range t.Nodes {
		err := unrollTree(&subtree)
		if err != nil {
			return err
		}

		t.Nodes[i] = subtree
	}

	return nil
}

// newTree creates a Tree from the target files/directories.
func newTree(targets []restic.LazyFileMetadata) (*Tree, error) {
	debug.Log("targets: %v", targets)
	tree := &Tree{}
	seen := make(map[string]struct{})
	for _, target := range targets {
		target = target.Clean()

		// skip duplicate targets
		if _, ok := seen[target.Path()]; ok {
			continue
		}
		seen[target.Path()] = struct{}{}

		err := tree.Add(target)
		if err != nil {
			return nil, err
		}
	}

	debug.Log("before unroll:\n%v", tree)
	err := unrollTree(tree)
	if err != nil {
		return nil, err
	}

	debug.Log("result:\n%v", tree)
	return tree, nil
}
