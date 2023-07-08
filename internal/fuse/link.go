//go:build darwin || freebsd || linux
// +build darwin freebsd linux

package fuse

import (
	"context"

	"github.com/anacrolix/fuse"
	"github.com/anacrolix/fuse/fs"
	"github.com/restic/restic/internal/restic"
)

// Statically ensure that *link implements the given interface
var _ = fs.NodeReadlinker(&link{})

type link struct {
	root  *Root
	node  *restic.Node
	inode uint64
}

func newLink(root *Root, inode uint64, node *restic.Node) (*link, error) {
	return &link{root: root, inode: inode, node: node}, nil
}

func (l *link) Readlink(_ context.Context, _ *fuse.ReadlinkRequest) (string, error) {
	return l.node.LinkTarget, nil
}

func (l *link) Attr(_ context.Context, a *fuse.Attr) error {
	a.Inode = l.inode
	a.Mode = l.node.Mode

	if !l.root.cfg.OwnerIsRoot {
		a.Uid = l.node.UID
		a.Gid = l.node.GID
	}
	a.Atime = l.node.AccessTime
	a.Ctime = l.node.ChangeTime
	a.Mtime = l.node.ModTime

	a.Nlink = uint32(l.node.Links)
	a.Size = uint64(len(l.node.LinkTarget))
	a.Blocks = (a.Size + blockSize - 1) / blockSize

	return nil
}

func (l *link) Listxattr(_ context.Context, req *fuse.ListxattrRequest, resp *fuse.ListxattrResponse) error {
	nodeToXattrList(l.node, req, resp)
	return nil
}

func (l *link) Getxattr(_ context.Context, req *fuse.GetxattrRequest, resp *fuse.GetxattrResponse) error {
	return nodeGetXattr(l.node, req, resp)
}
