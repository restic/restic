//go:build darwin || freebsd || linux

package fuse

import (
	"context"

	"github.com/anacrolix/fuse"
	"github.com/anacrolix/fuse/fs"
	"github.com/restic/restic/internal/data"
)

// Statically ensure that *other implements the given interface
var _ = fs.NodeForgetter(&other{})
var _ = fs.NodeReadlinker(&other{})

type other struct {
	root   *Root
	forget forgetFn
	node   *data.Node
	inode  uint64
}

func newOther(root *Root, forget forgetFn, inode uint64, node *data.Node) (*other, error) {
	return &other{root: root, forget: forget, inode: inode, node: node}, nil
}

func (l *other) Readlink(_ context.Context, _ *fuse.ReadlinkRequest) (string, error) {
	return l.node.LinkTarget, nil
}

func (l *other) Attr(_ context.Context, a *fuse.Attr) error {
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

func (l *other) Forget() {
	l.forget()
}
