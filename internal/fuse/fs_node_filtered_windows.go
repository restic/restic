package fuse

import (
	"github.com/billziss-gh/cgofuse/fuse"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/net/context"
)

type NamedSnapshotDir struct {
	name        string
	snapshotDir *FsNodeSnapshotDir
}

// FsNodeFiltered is a special node that filters and aggregates snapshots
// by a filter criteria e.g. by hostname or tags.
type FsNodeFiltered struct {
	root               *FsNodeRoot
	itemToSnapshots    map[string][]NamedSnapshotDir
	snapshotToItemName func(snapshot *restic.Snapshot) []string
}

var _ = FsNode(&FsNodeSnapshotDir{})

// NewFsNodeFiltered create a new FsNodeFiltered filesystem node.
// snapshotToItemName is a callback used to aggregate snapshots
// into different virtual folders.
func NewFsNodeFiltered(
	ctx context.Context, root *FsNodeRoot,
	snapshotToItemName func(snapshot *restic.Snapshot) []string,
) *FsNodeFiltered {

	node := &FsNodeFiltered{
		root:               root,
		itemToSnapshots:    make(map[string][]NamedSnapshotDir),
		snapshotToItemName: snapshotToItemName,
	}

	node.updateItems()
	return node
}

// updateItems updates the items inside this node from the available
// snapshots.
func (self *FsNodeFiltered) updateItems() {
	itemToSnapshots := make(map[string][]NamedSnapshotDir)

	for name, snapshot := range self.root.snapshotManager.snapshotByName {

		keys := self.snapshotToItemName(snapshot)

		for _, key := range keys {
			if _, found := itemToSnapshots[key]; !found {
				itemToSnapshots[key] = []NamedSnapshotDir{}
			}
		}

		snapNode, err := NewFsNodeSnapshotDirFromSnapshot(
			self.root.ctx, self.root, snapshot,
		)

		if err != nil {
			debug.Log(
				"failed to create FsNodeSnapshotDir: %v",
				err,
			)
			continue
		}

		for _, key := range keys {
			itemToSnapshots[key] = append(
				itemToSnapshots[key],
				NamedSnapshotDir{name: name, snapshotDir: snapNode},
			)
		}
	}

	self.itemToSnapshots = itemToSnapshots
}

// Readdir lists all items in the specified path. Results are returned
// through the given callback function.
func (self *FsNodeFiltered) Readdir(path []string, fill FsListItemCallback) {

	debug.Log("Readdir(%v)", path)

	pathLength := len(path)

	if pathLength == 0 {

		updated, _ := self.root.snapshotManager.updateSnapshots()

		if updated {
			self.updateItems()
		}

		fill(".", nil, 0)
		fill("..", nil, 0)

		for name := range self.itemToSnapshots {
			fill(name, &defaultDirectoryStat, 0)
		}

	} else if pathLength == 1 {

		head := path[0]

		if items, found := self.itemToSnapshots[head]; found {

			fill(".", nil, 0)
			fill("..", nil, 0)

			for _, item := range items {
				fill(item.name, &defaultDirectoryStat, 0)
			}
		}

	} else if pathLength > 1 {

		head := path[0]
		head2 := path[1]

		debug.Log("handle subtree %v", head)

		if items, found := self.itemToSnapshots[head]; found {
			for _, item := range items {
				if item.name == head2 {
					item.snapshotDir.Readdir(path[2:], fill)
				}
			}
		}

	}
}

// GetAttributes fetches the attributes of the specified file or directory.
func (self *FsNodeFiltered) GetAttributes(path []string, stat *fuse.Stat_t) bool {

	debug.Log("GetAttributes(%v)", path)

	pathLength := len(path)

	if pathLength < 1 {
		*stat = defaultDirectoryStat
		return true
	} else {

		head := path[0]

		if pathLength == 1 {

			if _, found := self.itemToSnapshots[head]; found {
				*stat = defaultDirectoryStat
				return true
			}

		} else if pathLength == 2 {

			head2 := path[1]

			if items, found := self.itemToSnapshots[head]; found {
				for _, item := range items {
					if item.name == head2 {
						*stat = defaultDirectoryStat
						return true
					}
				}
			}

		} else if pathLength > 2 {

			head2 := path[1]

			if items, found := self.itemToSnapshots[head]; found {
				for _, item := range items {
					if item.name == head2 {
						return item.snapshotDir.GetAttributes(path[2:], stat)
					}
				}
			}

		}
	}

	return false
}

// Open opens the file for the given path.
func (self *FsNodeFiltered) Open(path []string, flags int) (errc int, fh uint64) {

	pathLength := len(path)

	if pathLength < 1 {
		return -fuse.EISDIR, ^uint64(0)
	} else {

		head := path[0]

		if pathLength == 1 {
			if _, found := self.itemToSnapshots[head]; found {
				return -fuse.EISDIR, ^uint64(0)
			}
		} else if pathLength == 2 {

			head2 := path[1]

			if items, found := self.itemToSnapshots[head]; found {
				for _, item := range items {
					if item.name == head2 {
						return -fuse.EISDIR, ^uint64(0)
					}
				}
			}
		} else if pathLength > 2 {
			head2 := path[1]

			if items, found := self.itemToSnapshots[head]; found {
				for _, item := range items {
					if item.name == head2 {
						return item.snapshotDir.Open(path[2:], flags)
					}
				}
			}
		}
	}

	return -fuse.ENOENT, ^uint64(0)
}

// Read reads data to the given buffer from the specified file.
func (self *FsNodeFiltered) Read(path []string, buff []byte, ofst int64, fh uint64) (n int) {

	if len(path) > 2 {

		head := path[0]
		head2 := path[1]

		if items, found := self.itemToSnapshots[head]; found {
			for _, item := range items {
				if item.name == head2 {
					return item.snapshotDir.Read(path[2:], buff, ofst, fh)
				}
			}
		}

	}

	return -fuse.ENOENT
}
