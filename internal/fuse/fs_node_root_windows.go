package fuse

import (
	"github.com/billziss-gh/cgofuse/fuse"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/net/context"
)

// FsNodeRoot is the root node of the fuse filesystem.
type FsNodeRoot struct {
	ctx             context.Context
	repo            restic.Repository
	cfg             Config
	snapshotManager SnapshotManager
	blobCache       *blobCache
	entries         map[string]FsNode
}

var _ = FsNode(&FsNodeRoot{})

// NewNodeRoot creates a new FsNodeRoot for the given repository.
func NewFsNodeRoot(
	ctx context.Context, repo restic.Repository,
	cfg Config, snapshotManager SnapshotManager,
) *FsNodeRoot {

	root := &FsNodeRoot{
		ctx:             ctx,
		repo:            repo,
		cfg:             cfg,
		snapshotManager: snapshotManager,
		blobCache:       newBlobCache(blobCacheSize),
	}

	entries := map[string]FsNode{
		"snapshots": NewSnapshotsDir(ctx, root),
		"hosts": NewFsNodeFiltered(ctx, root, func(snapshot *restic.Snapshot) []string {
			return []string{snapshot.Hostname}
		}),
		"ids": NewFsNodeFiltered(ctx, root, func(snapshot *restic.Snapshot) []string {
			return []string{snapshot.ID().Str()}
		}),
		"tags": NewFsNodeFiltered(ctx, root, func(snapshot *restic.Snapshot) []string {
			return snapshot.Tags
		}),
	}
	root.entries = entries

	return root
}

// Readdir lists all items in the specified path. Results are returned
// through the given callback function.
func (self *FsNodeRoot) Readdir(path []string, fill FsListItemCallback) {

	debug.Log("Readdir(%v)", path)

	if len(path) == 0 {
		fill(".", nil, 0)
		fill("..", nil, 0)

		for name := range self.entries {
			fill(name, &defaultDirectoryStat, 0)
		}
	} else {
		if entry, found := self.entries[path[0]]; found {
			entry.Readdir(path[1:], fill)
		}
	}
}

// GetAttributes fetches the attributes of the specified file or directory.
func (self *FsNodeRoot) GetAttributes(path []string, stat *fuse.Stat_t) bool {

	if len(path) == 0 {
		*stat = defaultDirectoryStat
		return true
	}

	if entry, found := self.entries[path[0]]; found {
		return entry.GetAttributes(path[1:], stat)
	}

	return false
}

// Open opens the file for the given path.
func (self *FsNodeRoot) Open(path []string, flags int) (errc int, fh uint64) {

	lenPath := len(path)

	if lenPath <= 1 {
		return -fuse.EISDIR, ^uint64(0)
	}

	if entry, found := self.entries[path[0]]; found {
		return entry.Open(path[1:], flags)
	}

	return -fuse.ENOENT, ^uint64(0)
}

// Read reads data to the given buffer from the specified file.
func (self *FsNodeRoot) Read(path []string, buff []byte, ofst int64, fh uint64) (n int) {

	lenPath := len(path)

	if lenPath <= 1 {
		return -fuse.EISDIR
	}

	if entry, found := self.entries[path[0]]; found {
		return entry.Read(path[1:], buff, ofst, fh)
	}

	return -fuse.ENOENT
}
