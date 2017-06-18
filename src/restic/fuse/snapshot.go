// +build !openbsd
// +build !windows

package fuse

import (
	"fmt"
	"os"
	"sync"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"

	"restic"
	"restic/debug"
	"restic/repository"

	"golang.org/x/net/context"
)

// BlobSizeCache caches the size of blobs in the repo.
type BlobSizeCache struct {
	m map[restic.ID]uint
}

// NewBlobSizeCache returns a new blob size cache containing all entries from midx.
func NewBlobSizeCache(midx *repository.MasterIndex) *BlobSizeCache {
	m := make(map[restic.ID]uint, 1000)
	for _, idx := range midx.All() {
		for pb := range idx.Each(nil) {
			m[pb.ID] = pb.Length
		}
	}
	return &BlobSizeCache{
		m: m,
	}
}

// Lookup returns the size of the blob id.
func (c *BlobSizeCache) Lookup(id restic.ID) (size uint, found bool) {
	if c == nil {
		return 0, false
	}

	size, found = c.m[id]
	return size, found
}

// These lines statically ensure that a *SnapshotsDir implement the given
// interfaces; a misplaced refactoring of the implementation that breaks
// the interface will be catched by the compiler
var _ = fs.HandleReadDirAller(&SnapshotsDir{})
var _ = fs.NodeStringLookuper(&SnapshotsDir{})

type SnapshotsDir struct {
	repo        restic.Repository
	ownerIsRoot bool
	paths       []string
	tags        []string
	host        string

	blobsize *BlobSizeCache

	// knownSnapshots maps snapshot timestamp to the snapshot
	sync.Mutex
	knownSnapshots map[string]*restic.Snapshot
	processed      restic.IDSet
}

// NewSnapshotsDir returns a new dir object for the snapshots.
func NewSnapshotsDir(repo restic.Repository, ownerIsRoot bool, paths []string, tags []string, host string) *SnapshotsDir {
	debug.Log("fuse mount initiated")
	return &SnapshotsDir{
		repo:           repo,
		ownerIsRoot:    ownerIsRoot,
		paths:          paths,
		tags:           tags,
		host:           host,
		knownSnapshots: make(map[string]*restic.Snapshot),
		processed:      restic.NewIDSet(),
		blobsize:       NewBlobSizeCache(repo.Index().(*repository.MasterIndex)),
	}
}

func (sn *SnapshotsDir) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Inode = 0
	attr.Mode = os.ModeDir | 0555

	if !sn.ownerIsRoot {
		attr.Uid = uint32(os.Getuid())
		attr.Gid = uint32(os.Getgid())
	}
	debug.Log("attr is %v", attr)
	return nil
}

func (sn *SnapshotsDir) updateCache(ctx context.Context) error {
	debug.Log("called")
	sn.Lock()
	defer sn.Unlock()

	for id := range sn.repo.List(ctx, restic.SnapshotFile) {
		if sn.processed.Has(id) {
			debug.Log("skipping snapshot %v, already in list", id.Str())
			continue
		}

		debug.Log("found snapshot id %v", id.Str())
		snapshot, err := restic.LoadSnapshot(ctx, sn.repo, id)
		if err != nil {
			return err
		}

		// Filter snapshots we don't care for.
		if (sn.host != "" && sn.host != snapshot.Hostname) ||
			!snapshot.HasTags(sn.tags) ||
			!snapshot.HasPaths(sn.paths) {
			continue
		}

		timestamp := snapshot.Time.Format(time.RFC3339)
		for i := 1; ; i++ {
			if _, ok := sn.knownSnapshots[timestamp]; !ok {
				break
			}

			timestamp = fmt.Sprintf("%s-%d", snapshot.Time.Format(time.RFC3339), i)
		}

		debug.Log("  add %v as dir %v", id.Str(), timestamp)
		sn.knownSnapshots[timestamp] = snapshot
		sn.processed.Insert(id)
	}
	return nil
}

func (sn *SnapshotsDir) get(name string) (snapshot *restic.Snapshot, ok bool) {
	sn.Lock()
	snapshot, ok = sn.knownSnapshots[name]
	sn.Unlock()
	debug.Log("get(%s) -> %v %v", name, snapshot, ok)
	return snapshot, ok
}

func (sn *SnapshotsDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	debug.Log("called")
	err := sn.updateCache(ctx)
	if err != nil {
		return nil, err
	}

	sn.Lock()
	defer sn.Unlock()

	ret := make([]fuse.Dirent, 0)
	for timestamp, snapshot := range sn.knownSnapshots {
		ret = append(ret, fuse.Dirent{
			Inode: inodeFromBackendID(*snapshot.ID()),
			Type:  fuse.DT_Dir,
			Name:  timestamp,
		})
	}

	debug.Log("  -> %d entries", len(ret))
	return ret, nil
}

func (sn *SnapshotsDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	debug.Log("Lookup(%s)", name)
	snapshot, ok := sn.get(name)

	if !ok {
		// We don't know about it, update the cache
		err := sn.updateCache(ctx)
		if err != nil {
			debug.Log("  Lookup(%s) -> err %v", name, err)
			return nil, err
		}
		snapshot, ok = sn.get(name)
		if !ok {
			// We still don't know about it, this time it really doesn't exist
			debug.Log("  Lookup(%s) -> not found", name)
			return nil, fuse.ENOENT
		}
	}

	return newDirFromSnapshot(ctx, sn.repo, snapshot, sn.ownerIsRoot, sn.blobsize)
}
