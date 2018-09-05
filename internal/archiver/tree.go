package archiver

import (
	"fmt"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
)

// Tree recursively defines how a snapshot should look like when
// archived.
//
// When `Path` is set, this is a leaf node and the contents of `Path` should be
// inserted at this point in the tree.
//
// The attribute `Root` is used to distinguish between files/dirs which have
// the same name, but live in a separate directory on the local file system.
//
// `FileInfoPath` is used to extract metadata for intermediate (=non-leaf)
// trees.
type Tree struct {
	Nodes        map[string]Tree
	Path         string // where the files/dirs to be saved are found
	FileInfoPath string // where the dir can be found that is not included itself, but its subdirs
	Root         string // parent directory of the tree
}

// pathComponents returns all path components of p. If a virtual directory
// (volume name on Windows) is added, virtualPrefix is set to true. See the
// tests for examples.
func pathComponents(fs fs.FS, p string, includeRelative bool) (components []string, virtualPrefix bool) {
	volume := fs.VolumeName(p)

	if !fs.IsAbs(p) {
		if !includeRelative {
			p = fs.Join(fs.Separator(), p)
		}
	}

	p = fs.Clean(p)

	for {
		dir, file := fs.Dir(p), fs.Base(p)

		if p == dir {
			break
		}

		components = append(components, file)
		p = dir
	}

	// reverse components
	for i := len(components)/2 - 1; i >= 0; i-- {
		opp := len(components) - 1 - i
		components[i], components[opp] = components[opp], components[i]
	}

	if volume != "" {
		// strip colon
		if len(volume) == 2 && volume[1] == ':' {
			volume = volume[:1]
		}

		components = append([]string{volume}, components...)
		virtualPrefix = true
	}

	return components, virtualPrefix
}

// rootDirectory returns the directory which contains the first element of target.
func rootDirectory(fs fs.FS, target string) string {
	if target == "" {
		return ""
	}

	if fs.IsAbs(target) {
		return fs.Join(fs.VolumeName(target), fs.Separator())
	}

	target = fs.Clean(target)
	pc, _ := pathComponents(fs, target, true)

	rel := "."
	for _, c := range pc {
		if c == ".." {
			rel = fs.Join(rel, c)
		}
	}

	return rel
}

// Add adds a new file or directory to the tree.
func (t *Tree) Add(fs fs.FS, path string) error {
	if path == "" {
		panic("invalid path (empty string)")
	}

	if t.Nodes == nil {
		t.Nodes = make(map[string]Tree)
	}

	pc, virtualPrefix := pathComponents(fs, path, false)
	if len(pc) == 0 {
		return errors.New("invalid path (no path components)")
	}

	name := pc[0]
	root := rootDirectory(fs, path)
	tree := Tree{Root: root}

	origName := name
	i := 0
	for {
		other, ok := t.Nodes[name]
		if !ok {
			break
		}

		i++
		if other.Root == root {
			tree = other
			break
		}

		// resolve conflict and try again
		name = fmt.Sprintf("%s-%d", origName, i)
		continue
	}

	if len(pc) > 1 {
		subroot := fs.Join(root, origName)
		if virtualPrefix {
			// use the original root dir if this is a virtual directory (volume name on Windows)
			subroot = root
		}
		err := tree.add(fs, path, subroot, pc[1:])
		if err != nil {
			return err
		}
		tree.FileInfoPath = subroot
	} else {
		tree.Path = path
	}

	t.Nodes[name] = tree
	return nil
}

// add adds a new target path into the tree.
func (t *Tree) add(fs fs.FS, target, root string, pc []string) error {
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
			t.Nodes[name] = Tree{Path: target}
			return nil
		}

		if tree.Path != "" {
			return errors.Errorf("path is already set for target %v", target)
		}
		tree.Path = target
		t.Nodes[name] = tree
		return nil
	}

	tree := Tree{}
	if other, ok := t.Nodes[name]; ok {
		tree = other
	}

	subroot := fs.Join(root, name)
	tree.FileInfoPath = subroot

	err := tree.add(fs, target, subroot, pc[1:])
	if err != nil {
		return err
	}
	t.Nodes[name] = tree

	return nil
}

func (t Tree) String() string {
	return formatTree(t, "")
}

// formatTree returns a text representation of the tree t.
func formatTree(t Tree, indent string) (s string) {
	for name, node := range t.Nodes {
		s += fmt.Sprintf("%v/%v, root %q, path %q, meta %q\n", indent, name, node.Root, node.Path, node.FileInfoPath)
		s += formatTree(node, indent+"    ")
	}
	return s
}

// unrollTree unrolls the tree so that only leaf nodes have Path set.
func unrollTree(f fs.FS, t *Tree) error {
	// if the current tree is a leaf node (Path is set) and has additional
	// nodes, add the contents of Path to the nodes.
	if t.Path != "" && len(t.Nodes) > 0 {
		debug.Log("resolve path %v", t.Path)
		entries, err := fs.ReadDirNames(f, t.Path)
		if err != nil {
			return err
		}

		for _, entry := range entries {
			if node, ok := t.Nodes[entry]; ok {
				if node.Path == "" {
					node.Path = f.Join(t.Path, entry)
					t.Nodes[entry] = node
					continue
				}

				if node.Path == f.Join(t.Path, entry) {
					continue
				}

				return errors.Errorf("tree unrollTree: collision on path, node %#v, path %q", node, f.Join(t.Path, entry))
			}
			t.Nodes[entry] = Tree{Path: f.Join(t.Path, entry)}
		}
		t.Path = ""
	}

	for i, subtree := range t.Nodes {
		err := unrollTree(f, &subtree)
		if err != nil {
			return err
		}

		t.Nodes[i] = subtree
	}

	return nil
}

// NewTree creates a Tree from the target files/directories.
func NewTree(fs fs.FS, targets []string) (*Tree, error) {
	debug.Log("targets: %v", targets)
	tree := &Tree{}
	seen := make(map[string]struct{})
	for _, target := range targets {
		target = fs.Clean(target)

		// skip duplicate targets
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}

		err := tree.Add(fs, target)
		if err != nil {
			return nil, err
		}
	}

	debug.Log("before unroll:\n%v", tree)
	err := unrollTree(fs, tree)
	if err != nil {
		return nil, err
	}

	debug.Log("result:\n%v", tree)
	return tree, nil
}
