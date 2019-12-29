package archiver

import (
	"context"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
	tomb "gopkg.in/tomb.v2"
)

// FutureTree is returned by Save and will return the data once it
// has been processed.
type FutureTree struct {
	ch  <-chan saveTreeResponse
	res saveTreeResponse
}

// Wait blocks until the data has been received or ctx is cancelled.
func (s *FutureTree) Wait(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case res, ok := <-s.ch:
		if ok {
			s.res = res
		}
	}
}

// Node returns the node.
func (s *FutureTree) Node() *restic.Node {
	return s.res.node
}

// Stats returns the stats for the file.
func (s *FutureTree) Stats() ItemStats {
	return s.res.stats
}

// FutureSubTree is returned by Save and will return the data once it
// has been processed.
type FutureSubTree struct {
	ch  <-chan saveSubTreeResponse
	res saveSubTreeResponse
}

// Wait blocks until the data has been received or ctx is cancelled.
func (s *FutureSubTree) Wait(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case res, ok := <-s.ch:
		if ok {
			s.res = res
		}
	}
}

// Node returns the ID.
func (s *FutureSubTree) ID() *restic.ID {
	return s.res.id
}

// Stats returns the stats for the file.
func (s *FutureSubTree) Stats() ItemStats {
	return s.res.stats
}

// TreeSaver concurrently saves incoming trees to the repo.
type TreeSaver struct {
	saveTree func(context.Context, *restic.Tree) (restic.ID, ItemStats, error)
	errFn    ErrorFunc

	ch   chan<- saveTreeJob
	chst chan<- saveSubTreeJob
	done <-chan struct{}
}

// NewTreeSaver returns a new tree saver. A worker pool with treeWorkers is
// started, it is stopped when ctx is cancelled.
func NewTreeSaver(ctx context.Context, t *tomb.Tomb, treeWorkers, subTreeWorkers uint, saveTree func(context.Context, *restic.Tree) (restic.ID, ItemStats, error), errFn ErrorFunc) *TreeSaver {
	ch := make(chan saveTreeJob)
	chst := make(chan saveSubTreeJob)

	s := &TreeSaver{
		ch:       ch,
		chst:     chst,
		done:     t.Dying(),
		saveTree: saveTree,
		errFn:    errFn,
	}

	for i := uint(0); i < treeWorkers; i++ {
		t.Go(func() error {
			return s.worker(t.Context(ctx), ch)
		})
	}

	for i := uint(0); i < subTreeWorkers; i++ {
		t.Go(func() error {
			return s.workerSubTree(t.Context(ctx), chst)
		})
	}

	return s
}

// Save stores the dir d and returns the data once it has been completed.
func (s *TreeSaver) Save(ctx context.Context, snPath string, node *restic.Node, subtrees []FutureSubTree) FutureTree {
	ch := make(chan saveTreeResponse, 1)

	// copy subtrees to avoid race condition
	subtreesCopy := make([]FutureSubTree, len(subtrees))
	copy(subtreesCopy, subtrees)
	job := saveTreeJob{
		snPath:   snPath,
		node:     node,
		subtrees: subtreesCopy,
		ch:       ch,
	}
	select {
	case s.ch <- job:
	case <-s.done:
		debug.Log("not saving tree, TreeSaver is done")
		close(ch)
		return FutureTree{ch: ch}
	case <-ctx.Done():
		debug.Log("not saving tree, context is cancelled")
		close(ch)
		return FutureTree{ch: ch}
	}

	return FutureTree{ch: ch}
}

// Save stores the dir d and returns the data once it has been completed.
func (s *TreeSaver) SaveSubTree(ctx context.Context, nodes []FutureNode) FutureSubTree {
	ch := make(chan saveSubTreeResponse, 1)
	// copy nodes to avoid race condition
	nodesCopy := make([]FutureNode, len(nodes))
	copy(nodesCopy, nodes)
	job := saveSubTreeJob{
		nodes: nodesCopy,
		ch:    ch,
	}
	select {
	case s.chst <- job:
	case <-s.done:
		debug.Log("not saving tree, TreeSaver is done")
		close(ch)
		return FutureSubTree{ch: ch}
	case <-ctx.Done():
		debug.Log("not saving tree, context is cancelled")
		close(ch)
		return FutureSubTree{ch: ch}
	}

	return FutureSubTree{ch: ch}
}

type saveTreeJob struct {
	snPath   string
	subtrees []FutureSubTree
	node     *restic.Node
	ch       chan<- saveTreeResponse
}

type saveTreeResponse struct {
	node  *restic.Node
	stats ItemStats
}

// save stores the nodes as a tree in the repo.
func (s *TreeSaver) save(ctx context.Context, snPath string, node *restic.Node, subtrees []FutureSubTree) (*restic.Node, ItemStats) {
	var stats ItemStats

	// If only one subtree is present, save in node.Subtree, else use node.Subtrees
	if len(subtrees) == 1 {
		fst := subtrees[0]
		fst.Wait(ctx)
		node.Subtree = fst.ID()
		stats.Add(fst.Stats())
	} else {
		for _, fst := range subtrees {
			fst.Wait(ctx)
			debug.Log("insert %v", fst.ID())
			node.Subtrees = append(node.Subtrees, fst.ID())
			stats.Add(fst.Stats())
		}
	}

	return node, stats
}

func (s *TreeSaver) worker(ctx context.Context, jobs <-chan saveTreeJob) error {
	for {
		var job saveTreeJob
		select {
		case <-ctx.Done():
			return nil
		case job = <-jobs:
		}

		node, stats := s.save(ctx, job.snPath, job.node, job.subtrees)

		job.ch <- saveTreeResponse{
			node:  node,
			stats: stats,
		}
	}
}

type saveSubTreeJob struct {
	nodes []FutureNode
	ch    chan<- saveSubTreeResponse
}

type saveSubTreeResponse struct {
	id    *restic.ID
	stats ItemStats
}

// save stores the nodes as a tree in the repo.
func (s *TreeSaver) saveSubTree(ctx context.Context, nodes []FutureNode) (*restic.ID, ItemStats, error) {
	var stats ItemStats

	tree := restic.NewTree()
	for _, fn := range nodes {
		fn.wait(ctx)

		// return the error if it wasn't ignored
		if fn.err != nil {
			debug.Log("err for %v: %v", fn.snPath, fn.err)
			fn.err = s.errFn(fn.target, fn.fi, fn.err)
			if fn.err == nil {
				// ignore error
				continue
			}

			return nil, stats, fn.err
		}

		// when the error is ignored, the node could not be saved, so ignore it
		if fn.node == nil {
			debug.Log("%v excluded: %v", fn.snPath, fn.target)
			continue
		}

		debug.Log("insert %v", fn.node.Name)
		err := tree.Insert(fn.node)
		if err != nil {
			return nil, stats, err
		}
	}

	id, treeStats, err := s.saveTree(ctx, tree)
	stats.Add(treeStats)
	if err != nil {
		return nil, stats, err
	}

	return &id, stats, nil
}

func (s *TreeSaver) workerSubTree(ctx context.Context, jobs <-chan saveSubTreeJob) error {
	for {
		var job saveSubTreeJob
		select {
		case <-ctx.Done():
			return nil
		case job = <-jobs:
		}

		id, stats, err := s.saveSubTree(ctx, job.nodes)
		if err != nil {
			debug.Log("error saving tree blob: %v", err)
			close(job.ch)
			return err
		}

		job.ch <- saveSubTreeResponse{
			id:    id,
			stats: stats,
		}
	}
}
