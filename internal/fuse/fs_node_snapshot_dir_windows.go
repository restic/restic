package fuse

import (
	"context"

	"github.com/billziss-gh/cgofuse/fuse"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
)

type FsNodeSnapshotDir struct {
	root  *FsNodeRoot
	items map[string]*restic.Node
	nodes map[string]*FsNodeSnapshotDir
}

var _ = FsNode(&FsNodeSnapshotDir{})

func newFsNodeSnapshotDir(
	ctx context.Context, root *FsNodeRoot, node *restic.Node,
) (*FsNodeSnapshotDir, error) {

	debug.Log("newFsNodeSnapshotDir %v (%v)", node.Name, node.Subtree)

	tree, err := root.repo.LoadTree(ctx, *node.Subtree)
	if err != nil {
		debug.Log("  error loading tree %v: %v", node.Subtree, err)
		return nil, err
	}

	items := make(map[string]*restic.Node)
	nodes := make(map[string]*FsNodeSnapshotDir)

	for _, node := range tree.Nodes {
		nodeName := cleanupNodeName(node.Name)
		items[nodeName] = node

		child, err := newFsNodeSnapshotDir(ctx, root, node)

		if err != nil {
			nodes[nodeName] = child
			debug.Log("newFsNodeSnapshotDir: child %v", nodeName)
		} else {
			debug.Log("newFsNodeSnapshotDir: error: %v", err)
		}
	}

	return &FsNodeSnapshotDir{
		root:  root,
		items: items,
		nodes: nodes,
	}, nil
}

func NewFsNodeSnapshotDirFromSnapshot(
	ctx context.Context, root *FsNodeRoot, snapshot *restic.Snapshot,
) (*FsNodeSnapshotDir, error) {

	debug.Log("NewFsNodeSnapshotDirFromSnapshot %v (%v)", snapshot.ID(), snapshot.Tree)

	tree, err := root.repo.LoadTree(ctx, *snapshot.Tree)
	if err != nil {
		debug.Log("  loadTree(%v) failed: %v", snapshot.ID(), err)
		return nil, err
	}

	items := make(map[string]*restic.Node)
	nodes := make(map[string]*FsNodeSnapshotDir)

	for _, n := range tree.Nodes {
		treeNodes, err := replaceSpecialNodes(ctx, root.repo, n)
		if err != nil {
			debug.Log("  replaceSpecialNodes(%v) failed: %v", n, err)
			return nil, err
		}

		for _, node := range treeNodes {
			nodeName := cleanupNodeName(node.Name)
			items[nodeName] = node

			child, err := newFsNodeSnapshotDir(root.ctx, root, node)

			if err != nil {
				nodes[nodeName] = child
				debug.Log("NewFsNodeSnapshotDirFromSnapshot: child %v", nodeName)
			} else {
				debug.Log("NewFsNodeSnapshotDirFromSnapshot: error: %v", err)
			}
		}
	}

	return &FsNodeSnapshotDir{
		root:  root,
		items: items,
		nodes: nodes,
	}, nil
}

func (self *FsNodeSnapshotDir) ListFiles(path []string, fill FsListItemCallback) {
	debug.Log("FsNodeSnapshotDir: ListFiles(%v)", path)

	// if len(path) > 0 {
	// 	if entry, found := self.entries[path[0]]; found {
	// 		entry.ListFiles(path[1:], fill)
	// 	}
	// }
}

func (self *FsNodeSnapshotDir) ListDirectories(path []string, fill FsListItemCallback) {

	debug.Log("FsNodeSnapshotDir: ListDirectories(%v)", path)

	fill(".", nil, 0)
	fill("..", nil, 0)

	// TODO: right now everything is a directory -> handle files

	if len(path) == 0 {
		for name, _ := range self.items {
			fill(name, &defaultDirectoryStat, 0)
		}
	} else {
		head := path[0]
		tail := path[1:]

		if item, itemOk := self.items[head]; itemOk {
			if _, nodeOk := self.nodes[head]; !nodeOk {

				debug.Log("FsNodeSnapshotDir: ListDirectories(%v): creating node for %v", path, head)
				child, err := newFsNodeSnapshotDir(self.root.ctx, self.root, item)

				if err != nil {
					self.nodes[head] = child
				} else {
					debug.Log("FsNodeSnapshotDir: ListDirectories error: %v", err)
					return
				}
			}

			self.nodes[head].ListDirectories(tail, fill)
		}
	}
}

func (self *FsNodeSnapshotDir) GetAttributes(path []string, stat *fuse.Stat_t) bool {

	debug.Log("FsNodeSnapshotDir: ListDirectories(%v)", path)

	if len(path) == 0 {
		*stat = defaultDirectoryStat
		return true
	}

	// TODO: right now everything is a directory -> handle files

	if node, found := self.nodes[path[0]]; found {
		return node.GetAttributes(path[1:], stat)
	}

	return false
}
