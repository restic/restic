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

// TreeSaver concurrently saves incoming trees to the repo.
type TreeSaver struct {
	saveTree func(context.Context, *restic.Tree) (restic.ID, ItemStats, error)
	errFn    ErrorFunc

	ch   chan<- saveTreeJob
	done <-chan struct{}
}

// NewTreeSaver returns a new tree saver. A worker pool with treeWorkers is
// started, it is stopped when ctx is cancelled.
func NewTreeSaver(ctx context.Context, t *tomb.Tomb, treeWorkers uint, saveTree func(context.Context, *restic.Tree) (restic.ID, ItemStats, error), errFn ErrorFunc) *TreeSaver {
	ch := make(chan saveTreeJob)

	s := &TreeSaver{
		ch:       ch,
		done:     t.Dying(),
		saveTree: saveTree,
		errFn:    errFn,
	}

	for i := uint(0); i < treeWorkers; i++ {
		t.Go(func() error {
			return s.worker(t.Context(ctx), ch)
		})
	}

	return s
}

// Save stores the dir d and returns the data once it has been completed.
func (s *TreeSaver) Save(ctx context.Context, snPath string, node *restic.Node, nodes []FutureNode) FutureTree {
	ch := make(chan saveTreeResponse, 1)
	job := saveTreeJob{
		snPath: snPath,
		node:   node,
		nodes:  nodes,
		ch:     ch,
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

type saveTreeJob struct {
	snPath string
	nodes  []FutureNode
	node   *restic.Node
	ch     chan<- saveTreeResponse
}

type saveTreeResponse struct {
	node  *restic.Node
	stats ItemStats
}

// save stores the nodes as a tree in the repo.
func (s *TreeSaver) save(ctx context.Context, snPath string, node *restic.Node, nodes []FutureNode) (*restic.Node, ItemStats, error) {
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

	node.Subtree = &id
	return node, stats, nil
}

func (s *TreeSaver) worker(ctx context.Context, jobs <-chan saveTreeJob) error {
	for {
		var job saveTreeJob
		select {
		case <-ctx.Done():
			return nil
		case job = <-jobs:
		}

		node, stats, err := s.save(ctx, job.snPath, job.node, job.nodes)
		if err != nil {
			debug.Log("error saving tree blob: %v", err)
			close(job.ch)
			return err
		}

		job.ch <- saveTreeResponse{
			node:  node,
			stats: stats,
		}
		close(job.ch)
	}
}
