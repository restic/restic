package fuse

import (
	"encoding/binary"
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
	repo     *repository.Repository
	children map[string]*restic.Node
	inode    uint64
}

func newDir(repo *repository.Repository, node *restic.Node) (*dir, error) {
	tree, err := restic.LoadTree(repo, node.Subtree)
	if err != nil {
		return nil, err
	}
	children := make(map[string]*restic.Node)
	for _, child := range tree.Nodes {
		children[child.Name] = child
	}

	return &dir{
		repo:     repo,
		children: children,
		inode:    node.Inode,
	}, nil
}

func newDirFromSnapshot(repo *repository.Repository, snapshot SnapshotWithId) (*dir, error) {
	tree, err := restic.LoadTree(repo, snapshot.Tree)
	if err != nil {
		return nil, err
	}
	children := make(map[string]*restic.Node)
	for _, node := range tree.Nodes {
		children[node.Name] = node
	}

	return &dir{
		repo:     repo,
		children: children,
		inode:    binary.BigEndian.Uint64(snapshot.ID),
	}, nil
}

func (d *dir) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Inode = d.inode
	a.Mode = os.ModeDir | 0555
	return nil
}

func (d *dir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	ret := make([]fuse.Dirent, 0, len(d.children))

	for _, node := range d.children {
		var typ fuse.DirentType
		switch {
		case node.Mode.IsDir():
			typ = fuse.DT_Dir
		case node.Mode.IsRegular():
			typ = fuse.DT_File
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
	child, ok := d.children[name]
	if !ok {
		return nil, fuse.ENOENT
	}
	switch {
	case child.Mode.IsDir():
		return newDir(d.repo, child)
	case child.Mode.IsRegular():
		return newFile(d.repo, child)
	default:
		return nil, fuse.ENOENT
	}
}
