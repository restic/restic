// +build !openbsd

package fuse

import (
	"os"
	"sync"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/repository"

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
	return nil
}

func (sn *SnapshotsDir) updateCache(ctx context.Context) error {
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
	return snapshot, ok
}

func (sn *SnapshotsDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
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

	return ret, nil
}

func (sn *SnapshotsDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	snapshot, ok := sn.get(name)

	if !ok {
		// We don't know about it, update the cache
		err := sn.updateCache(ctx)
		if err != nil {
			return nil, err
		}
		snapshot, ok = sn.get(name)
		if !ok {
			// We still don't know about it, this time it really doesn't exist
			return nil, fuse.ENOENT
		}
	}

	return newDirFromSnapshot(sn.repo, snapshot, sn.ownerIsRoot)
}
