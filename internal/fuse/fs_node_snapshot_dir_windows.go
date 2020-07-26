package fuse

import (
	"context"

	"github.com/billziss-gh/cgofuse/fuse"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
)

type FsNodeSnapshotDir struct {
	root        *FsNodeRoot
	files       map[string]*restic.Node
	directories map[string]*FsNodeSnapshotDir
}

var _ = FsNode(&FsNodeSnapshotDir{})

func newFsNodeSnapshotDir(
	ctx context.Context, root *FsNodeRoot, node *restic.Node,
) (*FsNodeSnapshotDir, error) {

	debug.Log("newFsNodeSnapshotDir %v (%v)", node.Name, node.Subtree)

	tree, err := root.repo.LoadTree(ctx, *node.Subtree)
	if err != nil {
		debug.Log("newFsNodeSnapshotDir error loading tree %v: %v", node.Subtree, err)
		return nil, err
	}

	files := make(map[string]*restic.Node)
	directories := make(map[string]*FsNodeSnapshotDir)

	debug.Log("newFsNodeSnapshotDir tree nodes %v", len(tree.Nodes))

	for _, node := range tree.Nodes {

		debug.Log("newFsNodeSnapshotDir handling node %v", node.Name)

		nodeName := cleanupNodeName(node.Name)

		switch node.Type {
		case "file":
			files[nodeName] = node
		case "dir":
			child, err := newFsNodeSnapshotDir(ctx, root, node)

			if err != nil {
				return nil, err
			}

			directories[nodeName] = child
		}
	}

	return &FsNodeSnapshotDir{
		root:        root,
		files:       files,
		directories: directories,
	}, nil
}

func NewFsNodeSnapshotDirFromSnapshot(
	ctx context.Context, root *FsNodeRoot, snapshot *restic.Snapshot,
) (*FsNodeSnapshotDir, error) {

	debug.Log("NewFsNodeSnapshotDirFromSnapshot for id %v (tree %v)", snapshot.ID(), snapshot.Tree)

	tree, err := root.repo.LoadTree(ctx, *snapshot.Tree)
	if err != nil {
		debug.Log("NewFsNodeSnapshotDirFromSnapshot loadTree(%v) failed: %v", snapshot.ID(), err)
		return nil, err
	}

	files := make(map[string]*restic.Node)
	directories := make(map[string]*FsNodeSnapshotDir)

	for _, n := range tree.Nodes {

		treeNodes, err := replaceSpecialNodes(ctx, root.repo, n)
		if err != nil {
			debug.Log("  replaceSpecialNodes(%v) failed: %v", n, err)
			return nil, err
		}

		for _, node := range treeNodes {

			nodeName := cleanupNodeName(node.Name)

			switch node.Type {
			case "file":
				files[nodeName] = node
			case "dir":
				child, err := newFsNodeSnapshotDir(ctx, root, node)

				if err != nil {
					return nil, err
				}

				directories[nodeName] = child
				debug.Log("NewFsNodeSnapshotDirFromSnapshot: child %v", nodeName)
			}
		}
	}

	return &FsNodeSnapshotDir{
		root:        root,
		files:       files,
		directories: directories,
	}, nil
}

func (self *FsNodeSnapshotDir) Readdir(path []string, fill FsListItemCallback) {

	debug.Log("FsNodeSnapshotDir: Readdir(%v)", path)

	fill(".", nil, 0)
	fill("..", nil, 0)

	if len(path) == 0 {
		for name, _ := range self.directories {
			fill(name, &defaultDirectoryStat, 0)
		}

		for name, file := range self.files {
			fileStat := fuse.Stat_t{}
			nodeToStat(file, &fileStat)
			fill(name, &fileStat, 0)
		}
	} else {
		head := path[0]
		tail := path[1:]

		if dir, found := self.directories[head]; found {
			dir.Readdir(tail, fill)
		}
	}
}

func (self *FsNodeSnapshotDir) GetAttributes(path []string, stat *fuse.Stat_t) bool {

	debug.Log("FsNodeSnapshotDir: ListDirectories(%v)", path)

	if len(path) == 0 {
		*stat = defaultDirectoryStat
		return true
	}

	head := path[0]

	if file, found := self.files[head]; found {
		nodeToStat(file, stat)
		return true
	}

	if dir, found := self.directories[head]; found {
		tail := path[1:]
		return dir.GetAttributes(tail, stat)
	}

	return false
}

func nodeToStat(node *restic.Node, stat *fuse.Stat_t) {
	switch node.Type {
	case "dir":
		stat.Mode = fuse.S_IFDIR | 0555
	case "file":
		stat.Mode = 0555
		stat.Size = int64(node.Size)
	}
}
