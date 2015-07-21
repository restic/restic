package fuse

import (
	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/restic/restic"
	"github.com/restic/restic/repository"
	"golang.org/x/net/context"
)

// Statically ensure that *file implements the given interface
var _ = fs.NodeReadlinker(&link{})

type link struct {
	node *restic.Node
}

func newLink(repo *repository.Repository, node *restic.Node) (*link, error) {
	return &link{node: node}, nil
}

func (l *link) Readlink(ctx context.Context, req *fuse.ReadlinkRequest) (string, error) {
	return l.node.LinkTarget, nil
}

func (l *link) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Inode = l.node.Inode
	a.Mode = l.node.Mode
	return nil
}
