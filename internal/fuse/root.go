//go:build darwin || freebsd || linux
// +build darwin freebsd linux

package fuse

import (
	"os"

	"github.com/restic/restic/internal/bloblru"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"

	"github.com/anacrolix/fuse/fs"
)

// Config holds settings for the fuse mount.
type Config struct {
	OwnerIsRoot   bool
	Filter        restic.SnapshotFilter
	TimeTemplate  string
	PathTemplates []string
}

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

// Size of the blob cache. TODO: make this configurable.
const blobCacheSize = 64 << 20

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
		cfg.PathTemplates = []string{
			"ids/%i",
			"snapshots/%T",
			"hosts/%h/%T",
			"tags/%t/%T",
		}
	}

	root.SnapshotsDir = NewSnapshotsDir(root, rootInode, rootInode, NewSnapshotsDirStructure(root, cfg.PathTemplates, cfg.TimeTemplate), "")

	return root
}

// Root is just there to satisfy fs.Root, it returns itself.
func (r *Root) Root() (fs.Node, error) {
	debug.Log("Root()")
	return r, nil
}
