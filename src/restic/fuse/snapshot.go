// +build !openbsd
// +build !windows

package fuse

import (
	"os"
	"sync"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"

	"restic"
	"restic/backend"
	"restic/debug"
	"restic/repository"

	"golang.org/x/net/context"
)

type SnapshotWithId struct {
	*restic.Snapshot
	backend.ID
}

// These lines statically ensure that a *SnapshotsDir implement the given
// interfaces; a misplaced refactoring of the implementation that breaks
// the interface will be catched by the compiler
var _ = fs.HandleReadDirAller(&SnapshotsDir{})
var _ = fs.NodeStringLookuper(&SnapshotsDir{})

type SnapshotsDir struct {
	repo        *repository.Repository
	ownerIsRoot bool

	// knownSnapshots maps snapshot timestamp to the snapshot
	sync.RWMutex
	knownSnapshots map[string]SnapshotWithId
}

func NewSnapshotsDir(repo *repository.Repository, ownerIsRoot bool) *SnapshotsDir {
	debug.Log("NewSnapshotsDir", "fuse mount initiated")
	return &SnapshotsDir{
		repo:           repo,
		knownSnapshots: make(map[string]SnapshotWithId),
		ownerIsRoot:    ownerIsRoot,
	}
}

func (sn *SnapshotsDir) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Inode = 0
	attr.Mode = os.ModeDir | 0555

	if !sn.ownerIsRoot {
		attr.Uid = uint32(os.Getuid())
		attr.Gid = uint32(os.Getgid())
	}
	debug.Log("SnapshotsDir.Attr", "attr is %v", attr)
	return nil
}

func (sn *SnapshotsDir) updateCache(ctx context.Context) error {
	debug.Log("SnapshotsDir.updateCache", "called")
	sn.Lock()
	defer sn.Unlock()

	for id := range sn.repo.List(backend.Snapshot, ctx.Done()) {
		snapshot, err := restic.LoadSnapshot(sn.repo, id)
		if err != nil {
			return err
		}
		sn.knownSnapshots[snapshot.Time.Format(time.RFC3339)] = SnapshotWithId{snapshot, id}
	}
	return nil
}

func (sn *SnapshotsDir) get(name string) (snapshot SnapshotWithId, ok bool) {
	sn.RLock()
	snapshot, ok = sn.knownSnapshots[name]
	sn.RUnlock()
	debug.Log("SnapshotsDir.get", "get(%s) -> %v %v", name, snapshot, ok)
	return snapshot, ok
}

func (sn *SnapshotsDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	debug.Log("SnapshotsDir.ReadDirAll", "called")
	err := sn.updateCache(ctx)
	if err != nil {
		return nil, err
	}

	sn.RLock()
	defer sn.RUnlock()

	ret := make([]fuse.Dirent, 0)
	for _, snapshot := range sn.knownSnapshots {
		ret = append(ret, fuse.Dirent{
			Inode: inodeFromBackendId(snapshot.ID),
			Type:  fuse.DT_Dir,
			Name:  snapshot.Time.Format(time.RFC3339),
		})
	}

	debug.Log("SnapshotsDir.ReadDirAll", "  -> %d entries", len(ret))
	return ret, nil
}

func (sn *SnapshotsDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	debug.Log("SnapshotsDir.updateCache", "Lookup(%s)", name)
	snapshot, ok := sn.get(name)

	if !ok {
		// We don't know about it, update the cache
		err := sn.updateCache(ctx)
		if err != nil {
			debug.Log("SnapshotsDir.updateCache", "  Lookup(%s) -> err %v", name, err)
			return nil, err
		}
		snapshot, ok = sn.get(name)
		if !ok {
			// We still don't know about it, this time it really doesn't exist
			debug.Log("SnapshotsDir.updateCache", "  Lookup(%s) -> not found", name)
			return nil, fuse.ENOENT
		}
	}

	return newDirFromSnapshot(sn.repo, snapshot, sn.ownerIsRoot)
}
