package restic

import (
	"context"
	"os"
	"path/filepath"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
)

// Restorer is used to restore a snapshot to a directory.
type Restorer struct {
	repo Repository
	sn   *Snapshot

	Error        func(dir string, node *Node, err error) error
	SelectFilter func(item string, dstpath string, node *Node) (selectedForRestore bool, childMayBeSelected bool)

	workers int
	cfire   chan *restoreTask
	cback   chan *restoreTask

	dst string
	ctx context.Context
	idx *HardlinkIndex

	dirTasks  []*restoreTask
	nodeTasks []*restoreTask
}

var restorerAbortOnAllErrors = func(str string, node *Node, err error) error { return err }

type restoreTask struct {
	res    *Restorer
	parent *restoreTask

	class    string
	node     *Node
	treeID   ID
	location string

	subdir int
	child  int

	err error
}

const taskTypeDir = "dir"
const taskTypeNode = "node"

func (task *restoreTask) sendback() {
	task.res.cback <- task
}

func (task *restoreTask) checkCompeleted() error {
	if task.class != taskTypeDir {
		return nil
	}

	if task.child == 0 && task.subdir == 0 {
		if task.node != nil {
			res, node, dir := task.res, task.node, task.location
			ctx, repo, dst, idx := res.ctx, res.repo, res.dst, res.idx

			dstPath := filepath.Join(dst, dir)
			// TODO Error Handler??
			node.CreateAt(ctx, dstPath, repo, idx)
			node.RestoreTimestamps(dstPath)
		}

		if task.parent != nil {
			task.parent.subdir--
			return task.parent.checkCompeleted()
		}
	}
	return nil
}

func (task *restoreTask) restoreNodeTo() {
	res, node, dir := task.res, task.node, task.location
	ctx, repo, dst, idx := res.ctx, res.repo, res.dst, res.idx

	debug.Log("node %v, dir %v, dst %v", node.Name, dir, dst)
	dstPath := filepath.Join(dst, dir, node.Name)

	err := node.CreateAt(ctx, dstPath, repo, idx)
	if err != nil {
		debug.Log("node.CreateAt(%s) error %v", dstPath, err)
	}

	if err != nil && os.IsNotExist(errors.Cause(err)) {
		debug.Log("create intermediate paths")

		// Create parent directories and retry
		err = fs.MkdirAll(filepath.Dir(dstPath), 0700)
		if err == nil || os.IsExist(errors.Cause(err)) {
			err = node.CreateAt(ctx, dstPath, res.repo, idx)
		}
	}

	if err != nil {
		debug.Log("error %v", err)
		err = res.Error(dstPath, node, err)
		if err != nil {
			task.err = err
		}
	}

	debug.Log("successfully restored %v", node.Name)
	task.err = nil
}

func (task *restoreTask) run() {
	defer task.sendback()

	if task.class == taskTypeNode {
		task.restoreNodeTo()
	}
}

// NewRestorer creates an extend restorer from basic restorer object.
func NewRestorer(repo Repository, id ID, workers int) (*Restorer, error) {
	r := &Restorer{
		repo:         repo,
		Error:        restorerAbortOnAllErrors,
		SelectFilter: func(string, string, *Node) (bool, bool) { return true, true },
		workers:      workers,
	}

	var err error
	r.sn, err = LoadSnapshot(context.TODO(), repo, id)
	if err != nil {
		return nil, err
	}

	return r, nil
}

// restore worker
func restoreWorker(res *Restorer) {
	for {
		task, ok := <-res.cfire
		if !ok {
			return
		}
		task.run()
	}
}

func newNodeTask(res *Restorer, parent *restoreTask, location string, node *Node) *restoreTask {
	return &restoreTask{
		res:      res,
		parent:   parent,
		class:    taskTypeNode,
		location: location,
		node:     node,
	}
}

func newDirTask(res *Restorer, parent *restoreTask, location string, node *Node, treeID ID) *restoreTask {
	return &restoreTask{
		res:      res,
		parent:   parent,
		class:    taskTypeDir,
		location: location,
		node:     node,
		treeID:   treeID,
	}
}

func (res *Restorer) restoreDir(task *restoreTask) error {
	ctx, dst := res.ctx, res.dst
	dir, treeID := task.location, task.treeID

	tree, err := res.repo.LoadTree(ctx, treeID)

	if err != nil {
		return res.Error(dir, nil, err)
	}

	for _, node := range tree.Nodes {
		nodeName := filepath.Base(filepath.Join(string(filepath.Separator), node.Name))
		if nodeName != node.Name {
			debug.Log("node %q has invalid name %q", node.Name, nodeName)
			err := res.Error(dir, node, errors.New("node has invalid name"))
			if err != nil {
				return err
			}
			continue
		}

		selectedForRestore, childMayBeSelected := res.SelectFilter(filepath.Join(dir, node.Name),
			filepath.Join(dst, dir, node.Name), node)
		debug.Log("SelectFilter returned %v %v", selectedForRestore, childMayBeSelected)

		if node.Type == taskTypeDir && childMayBeSelected {
			if node.Subtree == nil {
				return errors.Errorf("Dir without subtree in tree %v", treeID.Str())
			}

			subp := filepath.Join(dir, node.Name)

			if selectedForRestore {
				res.addDirTask(task, subp, node, *node.Subtree)
			} else {
				res.addDirTask(task, subp, nil, *node.Subtree)
			}

			task.subdir++
			continue
		}

		if selectedForRestore {
			res.addNodeTask(task, dir, node)
			task.child++
			continue
		}
	}
	return nil
}

func (res *Restorer) restoreMain() error {
	available := res.workers
	var tasks int
	var task *restoreTask

	for {
		if available > 0 {
			tasks = len(res.nodeTasks)
			if tasks > 0 {
				task, res.nodeTasks = res.nodeTasks[tasks-1], res.nodeTasks[:tasks-1]
				res.cfire <- task
				available--
				continue
			}

			tasks = len(res.dirTasks)
			if tasks > 0 {
				task, res.dirTasks = res.dirTasks[tasks-1], res.dirTasks[:tasks-1]
				err := res.restoreDir(task)
				if err != nil {
					return err
				}
				continue
			}
		}

		if available == res.workers {
			return nil
		}

		task, ok := <-res.cback

		if !ok {
			return nil
		}
		available++

		if task.err != nil {
			return task.err
		}

		if task.parent != nil {
			if task.class == taskTypeNode {
				task.parent.child--
				if err := task.parent.checkCompeleted(); err != nil {
					return err
				}
			}
		}
	}
}

func (res *Restorer) addNodeTask(parent *restoreTask, dir string, node *Node) *restoreTask {
	task := newNodeTask(res, parent, dir, node)
	res.nodeTasks = append(res.nodeTasks, task)
	return task
}

func (res *Restorer) addDirTask(parent *restoreTask, dir string, node *Node, treeID ID) *restoreTask {
	task := newDirTask(res, parent, dir, node, treeID)
	res.dirTasks = append(res.dirTasks, task)
	return task
}

// RestoreTo creates the directories and files in the snapshot below dst.
// Before an item is created, res.Filter is called.
func (res *Restorer) RestoreTo(ctx context.Context, dst string) error {
	res.ctx = ctx
	res.idx = NewHardlinkIndex()
	res.dst = dst

	res.cfire = make(chan *restoreTask)
	res.cback = make(chan *restoreTask)

	res.dirTasks = make([]*restoreTask, 0, 100)
	res.nodeTasks = make([]*restoreTask, 0, 100)

	res.addDirTask(nil, string(filepath.Separator), nil, *res.sn.Tree)

	// start worker pool
	for i := 0; i < res.workers; i++ {
		go restoreWorker(res)
	}

	err := res.restoreMain()

	close(res.cfire)
	close(res.cback)

	res.cfire = nil
	res.cback = nil

	return err
}

// Snapshot returns the snapshot this restorer is configured to use.
func (res *Restorer) Snapshot() *Snapshot {
	return res.sn
}
