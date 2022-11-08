//go:build darwin || freebsd || linux
// +build darwin freebsd linux

package fuse

import (
	"context"
	"os"

	"github.com/restic/restic/internal/bloblru"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"

	"bazil.org/fuse/fs"
	"golang.org/x/sync/semaphore"
	"golang.org/x/sync/singleflight"
)

// Config holds settings for the fuse mount.
type Config struct {
	OwnerIsRoot   bool
	Hosts         []string
	Tags          []restic.TagList
	Paths         []string
	TimeTemplate  string
	PathTemplates []string
}

// Root is the root node of the fuse mount of a repository.
type Root struct {
	repo      restic.Repository
	cfg       Config
	blobCache *bloblru.Cache
	single    singleflight.Group
	readahead *semaphore.Weighted

	*SnapshotsDir

	uid, gid uint32
}

// ensure that *Root implements these interfaces
var _ = fs.HandleReadDirAller(&Root{})
var _ = fs.NodeStringLookuper(&Root{})

const rootInode = 1

const (
	// Size of the blob cache. TODO: make this configurable?
	blobCacheSize = 64 << 20

	// Max. memory used by readahead. This must be less than the size of the
	// blob cache, since the purpose of readahead is to pre-fill that cache.
	// This also implicitly controls the number of goroutines used.
	readaheadSize = 16 << 20
)

// NewRoot initializes a new root node from a repository.
func NewRoot(repo restic.Repository, cfg Config) *Root {
	debug.Log("NewRoot(), config %v", cfg)

	root := &Root{
		repo:      repo,
		cfg:       cfg,
		blobCache: bloblru.New(blobCacheSize),
		readahead: semaphore.NewWeighted(readaheadSize),
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

func (r *Root) readBlob(ctx context.Context, id restic.ID) ([]byte, error) {
	blob, ok := r.blobCache.Get(id)
	if ok {
		return blob, nil
	}

	b, err, _ := r.single.Do(string(id[:]), func() (interface{}, error) {
		return r.repo.LoadBlob(ctx, restic.DataBlob, id, nil)
	})
	if err != nil {
		debug.Log("readBlob(%v) failed: %v", id, err)
		return nil, unwrapCtxCanceled(err)
	}

	blob = b.([]byte)
	r.blobCache.Add(id, blob)
	return blob, nil
}

// readBlobAhead may spawn a goroutine that fetches a blob
// and stores it in r.blobCache. It does nothing if the blob
// is in the cache or no memory is available.
func (r *Root) readBlobAhead(id restic.ID, size int64) {
	if _, ok := r.blobCache.Get(id); ok {
		return
	}

	// Sloppy estimate of the memory cost of a DoChan call.
	const (
		costGoroutine = 2048
		costOther     = 512 // singleflight data structures
	)
	mem := size + costGoroutine + costOther
	if !r.readahead.TryAcquire(mem) {
		return
	}

	// We don't care about the result of this call.
	// DoChan creates a buffered channel, so not consuming it
	// does not cause a memory leak.
	r.single.DoChan(string(id[:]), func() (interface{}, error) {
		defer r.readahead.Release(mem)

		ctx := context.Background()
		blob, err := r.repo.LoadBlob(ctx, restic.DataBlob, id, nil)

		if err == nil {
			r.blobCache.Add(id, blob)
		}
		debug.Log("readBlobAhead(%v): %v", id, err)
		return blob, err
	})
}
