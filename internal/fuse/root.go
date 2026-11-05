//go:build darwin || freebsd || linux

package fuse

import (
	"os"

	"github.com/restic/restic/internal/bloblru"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"

	"github.com/anacrolix/fuse/fs"
)

// Root is the root node of the fuse mount of a repository.
type Root struct {
	repo      restic.Repository
	cfg       Config
	blobCache *bloblru.Cache

	*SnapshotsDir

	uid, gid uint32
}

// ensure that *Root implements these interfaces
var _ = fs.HandleReadDirAller(&Root{})
var _ = fs.NodeStringLookuper(&Root{})

const rootInode = 1

// NewRoot initializes a new root node from a repository.
func NewRoot(repo restic.Repository, cfg Config) *Root {
	debug.Log("NewRoot(), config %v", cfg)

	root := &Root{
		repo:      repo,
		cfg:       cfg,
		blobCache: bloblru.New(blobCacheSize),
	}

	if !cfg.OwnerIsRoot {
		root.uid = uint32(os.Getuid())
		root.gid = uint32(os.Getgid())
	}

	// set defaults, if PathTemplates is not set
	if len(cfg.PathTemplates) == 0 {
		cfg.PathTemplates = defaultPathTemplates
	}

	root.SnapshotsDir = NewSnapshotsDir(root, func() {}, rootInode, rootInode, NewSnapshotsDirStructure(repo, cfg.Filter, cfg.PathTemplates, cfg.TimeTemplate), "")

	return root
}

// Root is just there to satisfy fs.Root, it returns itself.
func (r *Root) Root() (fs.Node, error) {
	debug.Log("Root()")
	return r, nil
}
