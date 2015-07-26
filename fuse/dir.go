package fuse

import (
	"os"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"golang.org/x/net/context"

	"github.com/restic/restic"
	"github.com/restic/restic/repository"
)

// Statically ensure that *dir implement those interface
var _ = fs.HandleReadDirAller(&dir{})
var _ = fs.NodeStringLookuper(&dir{})

type dir struct {
	repo  *repository.Repository
	items map[string]*restic.Node
	inode uint64
}

func newDir(repo *repository.Repository, node *restic.Node) (*dir, error) {
	tree, err := restic.LoadTree(repo, *node.Subtree)
	if err != nil {
		return nil, err
	}
	items := make(map[string]*restic.Node)
	for _, node := range tree.Nodes {
		items[node.Name] = node
	}

	return &dir{
		repo:  repo,
		items: items,
		inode: node.Inode,
	}, nil
}

func newDirFromSnapshot(repo *repository.Repository, snapshot SnapshotWithId) (*dir, error) {
	tree, err := restic.LoadTree(repo, *snapshot.Tree)
	if err != nil {
		return nil, err
	}
	items := make(map[string]*restic.Node)
	for _, node := range tree.Nodes {
		items[node.Name] = node
	}

	return &dir{
		repo:  repo,
		items: items,
		inode: inodeFromBackendId(snapshot.ID),
	}, nil
}

func (d *dir) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Inode = d.inode
	a.Mode = os.ModeDir | 0555
	return nil
}

func (d *dir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
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
	node, ok := d.items[name]
	if !ok {
		return nil, fuse.ENOENT
	}
	switch node.Type {
	case "dir":
		return newDir(d.repo, node)
	case "file":
		return newFile(d.repo, node)
	case "symlink":
		return newLink(d.repo, node)
	default:
		return nil, fuse.ENOENT
	}
}
