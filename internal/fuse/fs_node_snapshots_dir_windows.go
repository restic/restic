package fuse

import (
	"sync"

	"github.com/billziss-gh/cgofuse/fuse"
	"github.com/restic/restic/internal/debug"
	"golang.org/x/net/context"
)

const snapshotDirLatestName = "latest"

type FsNodeSnapshotsDir struct {
	lock  sync.Mutex
	ctx   context.Context
	root  *FsNodeRoot
	nodes map[string]*FsNodeSnapshotDir
}

var _ = FsNode(&FsNodeSnapshotsDir{})

func NewSnapshotsDir(ctx context.Context, root *FsNodeRoot) *FsNodeSnapshotsDir {
	return &FsNodeSnapshotsDir{ctx: ctx, root: root, nodes: make(map[string]*FsNodeSnapshotDir)}
}

func (self *FsNodeSnapshotsDir) ListFiles(path []string, fill FsListItemCallback) {
	defer self.synchronize()()

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

	defer self.synchronize()()

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

		head := path[0]

		debug.Log("FsNodeSnapshotsDir: handle subtree %v", head)

		if snapshot, ok := self.root.snapshotManager.snapshotByName[head]; ok {

			if _, contained := self.nodes[head]; !contained {

				debug.Log("FsNodeSnapshotsDir: node not contained for: %v", head)

				node, err := NewFsNodeSnapshotDirFromSnapshot(self.ctx, self.root, snapshot)

				debug.Log("FsNodeSnapshotsDir: called NewFsNodeSnapshotDirFromSnapshot")
				debug.Log("FsNodeSnapshotsDir: err %v", err)
				debug.Log("FsNodeSnapshotsDir: head %v", head)
				debug.Log("FsNodeSnapshotsDir: len(nodes) %v", len(self.nodes))

				if err == nil {
					debug.Log("FsNodeSnapshotsDir: Before adding node for: %v", head)
					self.nodes[head] = node
					debug.Log("FsNodeSnapshotsDir: Added node for: %v", head)
				} else {
					debug.Log("FsNodeSnapshotsDir: Failed to create node for %v: %v", head, err.Error())
				}
			}

			debug.Log("FsNodeSnapshotsDir: finding node for: %v", head)

			if node, contained := self.nodes[head]; contained {
				debug.Log("FsNodeSnapshotsDir: ListDirectories for existing node %v", head)
				node.ListDirectories(path[1:], fill)
			} else {
				debug.Log("FsNodeSnapshotsDir: ListDirectories error for %v", head)
			}

		} else {
			debug.Log("FsNodeSnapshotsDir: Snapshot not found: %v", head)
		}
	}

	debug.Log("FsNodeSnapshotsDir: ListDirectories(%v) done", path)
}

func (self *FsNodeSnapshotsDir) GetAttributes(path []string, stat *fuse.Stat_t) bool {

	defer self.synchronize()()

	debug.Log("FsNodeSnapshotsDir: GetAttributes(%v)", path)

	pathLength := len(path)

	if pathLength < 1 {
		*stat = defaultDirectoryStat
		return true
	} else if pathLength == 1 {
		head := path[0]

		if head == snapshotDirLatestName && self.root.snapshotManager.snapshotNameLatest != "" {
			*stat = defaultDirectoryStat
			return true
		}

		if _, found := self.root.snapshotManager.snapshotByName[head]; found {
			*stat = defaultDirectoryStat
			return true
		}
	} else {
		head := path[0]
		if snapshotDir, ok := self.nodes[head]; ok {
			return snapshotDir.GetAttributes(path[1:], stat)
		} else {
			return false
		}
	}

	return false
}

func (self *FsNodeSnapshotsDir) synchronize() func() {
	self.lock.Lock()
	return func() {
		self.lock.Unlock()
	}
}
