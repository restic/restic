// +build darwin freebsd linux

package fuse

import (
	"os"
	"time"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"

	"bazil.org/fuse/fs"
)

// Config holds settings for the fuse mount.
type Config struct {
	OwnerIsRoot      bool
	Hosts            []string
	Tags             []restic.TagList
	Paths            []string
	SnapshotTemplate string
}

// Root is the root node of the fuse mount of a repository.
type Root struct {
	repo      restic.Repository
	cfg       Config
	inode     uint64
	snapshots restic.Snapshots
	blobCache *blobCache

	snCount   int
	lastCheck time.Time

	*MetaDir

	uid, gid uint32
}

// ensure that *Root implements these interfaces
var _ = fs.HandleReadDirAller(&Root{})
var _ = fs.NodeStringLookuper(&Root{})

const rootInode = 1

// Size of the blob cache. TODO: make this configurable.
const blobCacheSize = 64 << 20

// NewRoot initializes a new root node from a repository.
func NewRoot(repo restic.Repository, cfg Config) *Root {
	debug.Log("NewRoot(), config %v", cfg)

	root := &Root{
		repo:      repo,
		inode:     rootInode,
		cfg:       cfg,
		blobCache: newBlobCache(blobCacheSize),
	}

	if !cfg.OwnerIsRoot {
		root.uid = uint32(os.Getuid())
		root.gid = uint32(os.Getgid())
	}

	entries := map[string]fs.Node{
		"snapshots": NewSnapshotsDir(root, fs.GenerateDynamicInode(root.inode, "snapshots"), "", ""),
		"tags":      NewTagsDir(root, fs.GenerateDynamicInode(root.inode, "tags")),
		"hosts":     NewHostsDir(root, fs.GenerateDynamicInode(root.inode, "hosts")),
		"ids":       NewSnapshotsIDSDir(root, fs.GenerateDynamicInode(root.inode, "ids")),
	}

	root.MetaDir = NewMetaDir(root, rootInode, entries)

	return root
}

// Root is just there to satisfy fs.Root, it returns itself.
func (r *Root) Root() (fs.Node, error) {
	debug.Log("Root()")
	return r, nil
}
