//go:build darwin || freebsd || linux
// +build darwin freebsd linux

package fuse

import (
	"context"

	"github.com/anacrolix/fuse"
	"github.com/restic/restic/internal/restic"
)

type other struct {
	root  *Root
	node  *restic.Node
	inode uint64
}

func newOther(root *Root, inode uint64, node *restic.Node) (*other, error) {
	return &other{root: root, inode: inode, node: node}, nil
}

func (l *other) Readlink(ctx context.Context, req *fuse.ReadlinkRequest) (string, error) {
	return l.node.LinkTarget, nil
}

func (l *other) Attr(ctx context.Context, a *fuse.Attr) error {
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

	return nil
}
