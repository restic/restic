package fuse

import (
	"context"
	"errors"
	"sync"

	"github.com/billziss-gh/cgofuse/fuse"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
)

type FsNodeSnapshotDir struct {
	lock  sync.Mutex
	root  *FsNodeRoot
	items map[string]*restic.Node
	nodes map[string]*FsNodeSnapshotDir
}

var _ = FsNode(&FsNodeSnapshotDir{})

func newFsNodeSnapshotDir(
	ctx context.Context, root *FsNodeRoot, node *restic.Node,
) (*FsNodeSnapshotDir, error) {

	debug.Log("newFsNodeSnapshotDir %v (%v)", node.Name, node.Subtree)

	if node.Subtree == nil {
		return nil, errors.New("node.Subtree == nil")
	}

	tree, err := root.repo.LoadTree(ctx, *node.Subtree)
	if err != nil {
		debug.Log("  error loading tree %v: %v", node.Subtree, err)
		return nil, err
	}

	items := make(map[string]*restic.Node)
	nodes := make(map[string]*FsNodeSnapshotDir)

	debug.Log("newFsNodeSnapshotDir tree nodes %v", len(tree.Nodes))

	for _, node := range tree.Nodes {

		debug.Log("newFsNodeSnapshotDir handling node %v", node.Name)

		nodeName := cleanupNodeName(node.Name)
		child, err := newFsNodeSnapshotDir(ctx, root, node)

		if err != nil {
			//return nil, err
			continue
		}

		items[nodeName] = node
		nodes[nodeName] = child
		debug.Log("newFsNodeSnapshotDir: child %v", nodeName)
	}

	debug.Log("newFsNodeSnapshotDir %v (%v) DONE", node.Name, node.Subtree)

	return &FsNodeSnapshotDir{
		root:  root,
		items: items,
		nodes: nodes,
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
			child, err := newFsNodeSnapshotDir(root.ctx, root, node)

			if err != nil {
				debug.Log("NewFsNodeSnapshotDirFromSnapshot: error creating child %v", err.Error())
				//return nil, err
				continue
			}

			items[nodeName] = node
			nodes[nodeName] = child
			debug.Log("NewFsNodeSnapshotDirFromSnapshot: child %v", nodeName)
		}
	}

	result := &FsNodeSnapshotDir{
		root:  root,
		items: items,
		nodes: nodes,
	}

	debug.Log("NewFsNodeSnapshotDirFromSnapshot for id %v (tree %v) DONE", snapshot.ID(), snapshot.Tree)

	return result, nil
}

func (self *FsNodeSnapshotDir) ListFiles(path []string, fill FsListItemCallback) {

	defer self.synchronize()()

	debug.Log("FsNodeSnapshotDir: ListFiles(%v)", path)

	// if len(path) > 0 {
	// 	if entry, found := self.entries[path[0]]; found {
	// 		entry.ListFiles(path[1:], fill)
	// 	}
	// }
}

func (self *FsNodeSnapshotDir) ListDirectories(path []string, fill FsListItemCallback) {

	defer self.synchronize()()

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

	defer self.synchronize()()

	debug.Log("FsNodeSnapshotDir: ListDirectories(%v)", path)

	if len(path) == 0 {
		*stat = defaultDirectoryStat
		return true
	}

	// TODO: right now everything is a directory -> handle files
	head := path[0]
	tail := path[1:]

	if node, found := self.nodes[head]; found {
		return node.GetAttributes(tail, stat)
	}

	return false
}

func (self *FsNodeSnapshotDir) synchronize() func() {
	self.lock.Lock()
	return func() {
		self.lock.Unlock()
	}
}
