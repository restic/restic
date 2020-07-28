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
	return &FsNodeSnapshotsDir{ctx: ctx, root: root, nodes: make(map[string]*FsNodeSnapshotDir)}
}

func (self *FsNodeSnapshotsDir) Readdir(path []string, fill FsListItemCallback) {

	debug.Log("FsNodeSnapshotsDir: Readdir(%v)", path)

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
				node.Readdir(path[1:], fill)
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

	debug.Log("FsNodeSnapshotsDir: GetAttributes(%v)", path)

	pathLength := len(path)

	if pathLength < 1 {
		*stat = defaultDirectoryStat
		return true
	} else {

		head := path[0]

		if pathLength == 1 {
			if head == snapshotDirLatestName && self.root.snapshotManager.snapshotNameLatest != "" {
				*stat = defaultDirectoryStat
				return true
			}

			if _, found := self.root.snapshotManager.snapshotByName[head]; found {
				*stat = defaultDirectoryStat
				return true
			}
		} else {
			if snapshotDir, ok := self.nodes[head]; ok {
				return snapshotDir.GetAttributes(path[1:], stat)
			} else {
				return false
			}
		}
	}

	return false
}

func (self *FsNodeSnapshotsDir) Open(path []string, flags int) (errc int, fh uint64) {

	pathLength := len(path)

	if pathLength < 1 {
		return -fuse.EISDIR, ^uint64(0)
	} else {

		head := path[0]

		if pathLength == 1 {
			if head == snapshotDirLatestName && self.root.snapshotManager.snapshotNameLatest != "" {
				return -fuse.EISDIR, ^uint64(0)
			}

			if _, found := self.root.snapshotManager.snapshotByName[head]; found {
				return -fuse.EISDIR, ^uint64(0)
			}
		} else {
			if snapshotDir, ok := self.nodes[head]; ok {
				return snapshotDir.Open(path[1:], flags)
			}
		}
	}

	return -fuse.ENOENT, ^uint64(0)
}

func (self *FsNodeSnapshotsDir) Read(path []string, buff []byte, ofst int64, fh uint64) (n int) {

	pathLength := len(path)

	if pathLength < 1 {
		return -fuse.EISDIR
	} else {

		head := path[0]

		if pathLength == 1 {
			if head == snapshotDirLatestName && self.root.snapshotManager.snapshotNameLatest != "" {
				return -fuse.EISDIR
			}

			if _, found := self.root.snapshotManager.snapshotByName[head]; found {
				return -fuse.EISDIR
			}
		} else {
			if snapshotDir, ok := self.nodes[head]; ok {
				return snapshotDir.Read(path[1:], buff, ofst, fh)
			}
		}
	}

	return -fuse.ENOENT
}
