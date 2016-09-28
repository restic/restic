// +build !openbsd
// +build !windows

package fuse

import (
	"os"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"golang.org/x/net/context"

	"restic"
	"restic/debug"
)

// Statically ensure that *dir implement those interface
var _ = fs.HandleReadDirAller(&dir{})
var _ = fs.NodeStringLookuper(&dir{})

type dir struct {
	repo        restic.Repository
	items       map[string]*restic.Node
	inode       uint64
	node        *restic.Node
	ownerIsRoot bool
}

func newDir(repo restic.Repository, node *restic.Node, ownerIsRoot bool) (*dir, error) {
	debug.Log("new dir for %v (%v)", node.Name, node.Subtree.Str())
	tree, err := repo.LoadTree(*node.Subtree)
	if err != nil {
		debug.Log("  error loading tree %v: %v", node.Subtree.Str(), err)
		return nil, err
	}
	items := make(map[string]*restic.Node)
	for _, node := range tree.Nodes {
		items[node.Name] = node
	}

	return &dir{
		repo:        repo,
		node:        node,
		items:       items,
		inode:       node.Inode,
		ownerIsRoot: ownerIsRoot,
	}, nil
}

// replaceSpecialNodes replaces nodes with name "." and "/" by their contents.
// Otherwise, the node is returned.
func replaceSpecialNodes(repo restic.Repository, node *restic.Node) ([]*restic.Node, error) {
	if node.Type != "dir" || node.Subtree == nil {
		return []*restic.Node{node}, nil
	}

	if node.Name != "." && node.Name != "/" {
		return []*restic.Node{node}, nil
	}

	tree, err := repo.LoadTree(*node.Subtree)
	if err != nil {
		return nil, err
	}

	return tree.Nodes, nil
}

func newDirFromSnapshot(repo restic.Repository, snapshot SnapshotWithId, ownerIsRoot bool) (*dir, error) {
	debug.Log("new dir for snapshot %v (%v)", snapshot.ID.Str(), snapshot.Tree.Str())
	tree, err := repo.LoadTree(*snapshot.Tree)
	if err != nil {
		debug.Log("  loadTree(%v) failed: %v", snapshot.ID.Str(), err)
		return nil, err
	}
	items := make(map[string]*restic.Node)
	for _, n := range tree.Nodes {
		nodes, err := replaceSpecialNodes(repo, n)
		if err != nil {
			debug.Log("  replaceSpecialNodes(%v) failed: %v", n, err)
			return nil, err
		}

		for _, node := range nodes {
			items[node.Name] = node
		}
	}

	return &dir{
		repo: repo,
		node: &restic.Node{
			UID:        uint32(os.Getuid()),
			GID:        uint32(os.Getgid()),
			AccessTime: snapshot.Time,
			ModTime:    snapshot.Time,
			ChangeTime: snapshot.Time,
			Mode:       os.ModeDir | 0555,
		},
		items:       items,
		inode:       inodeFromBackendID(snapshot.ID),
		ownerIsRoot: ownerIsRoot,
	}, nil
}

func (d *dir) Attr(ctx context.Context, a *fuse.Attr) error {
	debug.Log("called")
	a.Inode = d.inode
	a.Mode = os.ModeDir | d.node.Mode

	if !d.ownerIsRoot {
		a.Uid = d.node.UID
		a.Gid = d.node.GID
	}
	a.Atime = d.node.AccessTime
	a.Ctime = d.node.ChangeTime
	a.Mtime = d.node.ModTime
	return nil
}

func (d *dir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	debug.Log("called")
	ret := make([]fuse.Dirent, 0, len(d.items))

	for _, node := range d.items {
		var typ fuse.DirentType
		switch node.Type {
		case "dir":
			typ = fuse.DT_Dir
		case "file":
			typ = fuse.DT_File
		case "symlink":
			typ = fuse.DT_Link
		}

		ret = append(ret, fuse.Dirent{
			Inode: node.Inode,
			Type:  typ,
			Name:  node.Name,
		})
	}

	return ret, nil
}

func (d *dir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	debug.Log("Lookup(%v)", name)
	node, ok := d.items[name]
	if !ok {
		debug.Log("  Lookup(%v) -> not found", name)
		return nil, fuse.ENOENT
	}
	switch node.Type {
	case "dir":
		return newDir(d.repo, node, d.ownerIsRoot)
	case "file":
		return newFile(d.repo, node, d.ownerIsRoot)
	case "symlink":
		return newLink(d.repo, node, d.ownerIsRoot)
	default:
		debug.Log("  node %v has unknown type %v", name, node.Type)
		return nil, fuse.ENOENT
	}
}
