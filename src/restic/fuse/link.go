// +build !openbsd
// +build !windows

package fuse

import (
	"restic"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"golang.org/x/net/context"
)

// Statically ensure that *file implements the given interface
var _ = fs.NodeReadlinker(&link{})

type link struct {
	node        *restic.Node
	ownerIsRoot bool
}

func newLink(repo restic.Repository, node *restic.Node, ownerIsRoot bool) (*link, error) {
	return &link{node: node, ownerIsRoot: ownerIsRoot}, nil
}

func (l *link) Readlink(ctx context.Context, req *fuse.ReadlinkRequest) (string, error) {
	return l.node.LinkTarget, nil
}

func (l *link) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Inode = l.node.Inode
	a.Mode = l.node.Mode

	if !l.ownerIsRoot {
		a.Uid = l.node.UID
		a.Gid = l.node.GID
	}
	a.Atime = l.node.AccessTime
	a.Ctime = l.node.ChangeTime
	a.Mtime = l.node.ModTime
	return nil
}
