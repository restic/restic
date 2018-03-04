// +build !openbsd
// +build !solaris
// +build !windows

package fuse

import (
	"time"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"

	"golang.org/x/net/context"

	"bazil.org/fuse/fs"
)

// Config holds settings for the fuse mount.
type Config struct {
	OwnerIsRoot      bool
	Host             string
	Tags             []restic.TagList
	Paths            []string
	SnapshotTemplate string
}

// Root is the root node of the fuse mount of a repository.
type Root struct {
	repo          restic.Repository
	cfg           Config
	inode         uint64
	snapshots     restic.Snapshots
	blobSizeCache *BlobSizeCache

	snCount   int
	lastCheck time.Time

	*MetaDir
}

// ensure that *Root implements these interfaces
var _ = fs.HandleReadDirAller(&Root{})
var _ = fs.NodeStringLookuper(&Root{})

const rootInode = 1

// NewRoot initializes a new root node from a repository.
func NewRoot(ctx context.Context, repo restic.Repository, cfg Config) (*Root, error) {
	debug.Log("NewRoot(), config %v", cfg)

	root := &Root{
		repo:          repo,
		inode:         rootInode,
		cfg:           cfg,
		blobSizeCache: NewBlobSizeCache(ctx, repo.Index()),
	}

	entries := map[string]fs.Node{
		"snapshots": NewSnapshotsDir(root, fs.GenerateDynamicInode(root.inode, "snapshots"), "", ""),
		"tags":      NewTagsDir(root, fs.GenerateDynamicInode(root.inode, "tags")),
		"hosts":     NewHostsDir(root, fs.GenerateDynamicInode(root.inode, "hosts")),
		"ids":       NewSnapshotsIDSDir(root, fs.GenerateDynamicInode(root.inode, "ids")),
	}

	root.MetaDir = NewMetaDir(root, rootInode, entries)

	return root, nil
}

// Root is just there to satisfy fs.Root, it returns itself.
func (r *Root) Root() (fs.Node, error) {
	debug.Log("Root()")
	return r, nil
}
