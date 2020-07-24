package fuse

import (
	"github.com/billziss-gh/cgofuse/fuse"
	"github.com/restic/restic/internal/debug"
	"golang.org/x/net/context"
)

const snapshotDirLatestName = "latest"

type FsNodeSnapshotsDir struct {
	ctx   context.Context
	root  *FsNodeRoot
	nodes map[string]*FsNodeSnapshotDir
}

var _ = FsNode(&FsNodeSnapshotsDir{})

func NewSnapshotsDir(ctx context.Context, root *FsNodeRoot) *FsNodeSnapshotsDir {
	return &FsNodeSnapshotsDir{ctx: ctx, root: root}
}

func (self *FsNodeSnapshotsDir) ListFiles(path []string, fill FsListItemCallback) {
	debug.Log("FsNodeSnapshotsDir: ListFiles(%v)", path)

	if len(path) > 0 {

		// if path[0] == self.snapshotManager.snapshotNameLatest {
		// 	//fill(snapshotDirLatestName, &defaultDirectoryStat, 0)
		// 	return
		// }

		// if entry, found := self.snapshotManager.snapshotByName[path[0]]; found {
		// 	entry.ListFiles(path[1:], fill)
		// 	return
		// }

	}
}

func (self *FsNodeSnapshotsDir) ListDirectories(path []string, fill FsListItemCallback) {

	debug.Log("FsNodeSnapshotsDir: ListDirectories(%v)", path)

	if len(path) == 0 {

		fill(".", nil, 0)
		fill("..", nil, 0)

		self.root.snapshotManager.updateSnapshots()

		if self.root.snapshotManager.snapshotNameLatest != "" {
			fill(snapshotDirLatestName, &defaultDirectoryStat, 0)
		}

		for name, _ := range self.root.snapshotManager.snapshotByName {
			fill(name, &defaultDirectoryStat, 0)
		}
	} else {

		if snapshot, ok := self.root.snapshotManager.snapshotByName[path[0]]; ok {
			node, err := NewFsNodeSnapshotDirFromSnapshot(self.ctx, self.root, snapshot)

			if err != nil {
				node.ListDirectories(path[1:], fill)
			} else {
				debug.Log("FsNodeSnapshotsDir: ListDirectories error: %v", err)
			}
		}
	}
}

func (self *FsNodeSnapshotsDir) GetAttributes(path []string, stat *fuse.Stat_t) bool {

	debug.Log("FsNodeSnapshotsDir: GetAttributes(%v)", path)

	if len(path) < 1 {
		*stat = defaultDirectoryStat
		return true
	}

	if path[0] == snapshotDirLatestName && self.root.snapshotManager.snapshotNameLatest != "" {
		*stat = defaultDirectoryStat
		return true
	}

	if _, found := self.root.snapshotManager.snapshotByName[path[0]]; found {
		*stat = defaultDirectoryStat
		return true
	}

	if len(path) > 1 {

		node.ListDirectories(path[1:], fill)
	}

	return false
}
