//go:build darwin || freebsd || linux
// +build darwin freebsd linux

package fuse

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/anacrolix/fuse"
	"github.com/anacrolix/fuse/fs"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
)

// Statically ensure that *dir implement those interface
var _ = fs.HandleReadDirAller(&dir{})
var _ = fs.NodeForgetter(&dir{})
var _ = fs.NodeGetxattrer(&dir{})
var _ = fs.NodeListxattrer(&dir{})
var _ = fs.NodeStringLookuper(&dir{})

type dir struct {
	root        *Root
	forget      forgetFn
	items       map[string]*data.Node
	inode       uint64
	parentInode uint64
	node        *data.Node
	m           sync.Mutex
	cache       treeCache
}

func cleanupNodeName(name string) string {
	return filepath.Base(name)
}

func newDir(root *Root, forget forgetFn, inode, parentInode uint64, node *data.Node) (*dir, error) {
	debug.Log("new dir for %v (%v)", node.Name, node.Subtree)

	return &dir{
		root:        root,
		forget:      forget,
		node:        node,
		inode:       inode,
		parentInode: parentInode,
		cache:       *newTreeCache(),
	}, nil
}

// returning a wrapped context.Canceled error will instead result in returning
// an input / output error to the user. Thus unwrap the error to match the
// expectations of bazil/fuse
func unwrapCtxCanceled(err error) error {
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	return err
}

// replaceSpecialNodes replaces nodes with name "." and "/" by their contents.
// Otherwise, the node is returned.
func replaceSpecialNodes(ctx context.Context, repo restic.BlobLoader, node *data.Node) ([]*data.Node, error) {
	if node.Type != data.NodeTypeDir || node.Subtree == nil {
		return []*data.Node{node}, nil
	}

	if node.Name != "." && node.Name != "/" {
		return []*data.Node{node}, nil
	}

	tree, err := data.LoadTree(ctx, repo, *node.Subtree)
	if err != nil {
		return nil, unwrapCtxCanceled(err)
	}

	return tree.Nodes, nil
}

func newDirFromSnapshot(root *Root, forget forgetFn, inode uint64, snapshot *data.Snapshot) (*dir, error) {
	debug.Log("new dir for snapshot %v (%v)", snapshot.ID(), snapshot.Tree)
	return &dir{
		root:   root,
		forget: forget,
		node: &data.Node{
			AccessTime: snapshot.Time,
			ModTime:    snapshot.Time,
			ChangeTime: snapshot.Time,
			Mode:       os.ModeDir | 0555,
			Subtree:    snapshot.Tree,
		},
		inode: inode,
		cache: *newTreeCache(),
	}, nil
}

func (d *dir) open(ctx context.Context) error {
	d.m.Lock()
	defer d.m.Unlock()

	if d.items != nil {
		return nil
	}

	debug.Log("open dir %v (%v)", d.node.Name, d.node.Subtree)

	tree, err := data.LoadTree(ctx, d.root.repo, *d.node.Subtree)
	if err != nil {
		debug.Log("  error loading tree %v: %v", d.node.Subtree, err)
		return unwrapCtxCanceled(err)
	}
	items := make(map[string]*data.Node)
	for _, n := range tree.Nodes {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		nodes, err := replaceSpecialNodes(ctx, d.root.repo, n)
		if err != nil {
			debug.Log("  replaceSpecialNodes(%v) failed: %v", n, err)
			return err
		}
		for _, node := range nodes {
			items[cleanupNodeName(node.Name)] = node
		}
	}
	d.items = items
	return nil
}

func (d *dir) Attr(_ context.Context, a *fuse.Attr) error {
	debug.Log("Attr()")
	a.Inode = d.inode
	a.Mode = os.ModeDir | d.node.Mode

	if !d.root.cfg.OwnerIsRoot {
		a.Uid = d.node.UID
		a.Gid = d.node.GID
	}
	a.Atime = d.node.AccessTime
	a.Ctime = d.node.ChangeTime
	a.Mtime = d.node.ModTime

	a.Nlink = d.calcNumberOfLinks()

	return nil
}

func (d *dir) calcNumberOfLinks() uint32 {
	// a directory d has 2 hardlinks + the number
	// of directories contained by d
	count := uint32(2)
	for _, node := range d.items {
		if node.Type == data.NodeTypeDir {
			count++
		}
	}
	return count
}

func (d *dir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	debug.Log("ReadDirAll()")
	err := d.open(ctx)
	if err != nil {
		return nil, err
	}
	ret := make([]fuse.Dirent, 0, len(d.items)+2)

	ret = append(ret, fuse.Dirent{
		Inode: d.inode,
		Name:  ".",
		Type:  fuse.DT_Dir,
	})

	ret = append(ret, fuse.Dirent{
		Inode: d.parentInode,
		Name:  "..",
		Type:  fuse.DT_Dir,
	})

	for _, node := range d.items {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		name := cleanupNodeName(node.Name)
		var typ fuse.DirentType
		switch node.Type {
		case data.NodeTypeDir:
			typ = fuse.DT_Dir
		case data.NodeTypeFile:
			typ = fuse.DT_File
		case data.NodeTypeSymlink:
			typ = fuse.DT_Link
		}

		ret = append(ret, fuse.Dirent{
			Inode: inodeFromNode(d.inode, node),
			Type:  typ,
			Name:  name,
		})
	}

	return ret, nil
}

func (d *dir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	debug.Log("Lookup(%v)", name)

	err := d.open(ctx)
	if err != nil {
		return nil, err
	}

	return d.cache.lookupOrCreate(name, func(forget forgetFn) (fs.Node, error) {
		node, ok := d.items[name]
		if !ok {
			debug.Log("  Lookup(%v) -> not found", name)
			return nil, syscall.ENOENT
		}
		inode := inodeFromNode(d.inode, node)
		switch node.Type {
		case data.NodeTypeDir:
			return newDir(d.root, forget, inode, d.inode, node)
		case data.NodeTypeFile:
			return newFile(d.root, forget, inode, node)
		case data.NodeTypeSymlink:
			return newLink(d.root, forget, inode, node)
		case data.NodeTypeDev, data.NodeTypeCharDev, data.NodeTypeFifo, data.NodeTypeSocket:
			return newOther(d.root, forget, inode, node)
		default:
			debug.Log("  node %v has unknown type %v", name, node.Type)
			return nil, syscall.ENOENT
		}
	})
}

func (d *dir) Listxattr(_ context.Context, req *fuse.ListxattrRequest, resp *fuse.ListxattrResponse) error {
	nodeToXattrList(d.node, req, resp)
	return nil
}

func (d *dir) Getxattr(_ context.Context, req *fuse.GetxattrRequest, resp *fuse.GetxattrResponse) error {
	return nodeGetXattr(d.node, req, resp)
}

func (d *dir) Forget() {
	d.forget()
}
