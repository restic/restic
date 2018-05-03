package archiver

import (
	"context"
	"sync"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
)

// FutureTree is returned by Save and will return the data once it
// has been processed.
type FutureTree struct {
	ch  <-chan saveTreeResponse
	res saveTreeResponse
}

func (s *FutureTree) wait() {
	res, ok := <-s.ch
	if ok {
		s.res = res
	}
}

// Node returns the node once it is available.
func (s *FutureTree) Node() *restic.Node {
	s.wait()
	return s.res.node
}

// Stats returns the stats for the file once they are available.
func (s *FutureTree) Stats() ItemStats {
	s.wait()
	return s.res.stats
}

// Err returns the error in case an error occurred.
func (s *FutureTree) Err() error {
	s.wait()
	return s.res.err
}

// TreeSaver concurrently saves incoming trees to the repo.
type TreeSaver struct {
	saveTree func(context.Context, *restic.Tree) (restic.ID, ItemStats, error)
	errFn    ErrorFunc

	ch chan<- saveTreeJob
	wg sync.WaitGroup
}

// NewTreeSaver returns a new tree saver. A worker pool with treeWorkers is
// started, it is stopped when ctx is cancelled.
func NewTreeSaver(ctx context.Context, treeWorkers uint, saveTree func(context.Context, *restic.Tree) (restic.ID, ItemStats, error), errFn ErrorFunc) *TreeSaver {
	ch := make(chan saveTreeJob)

	s := &TreeSaver{
		ch:       ch,
		saveTree: saveTree,
		errFn:    errFn,
	}

	for i := uint(0); i < treeWorkers; i++ {
		s.wg.Add(1)
		go s.worker(ctx, &s.wg, ch)
	}

	return s
}

// Save stores the dir d and returns the data once it has been completed.
func (s *TreeSaver) Save(ctx context.Context, snPath string, node *restic.Node, nodes []FutureNode) FutureTree {
	ch := make(chan saveTreeResponse, 1)
	s.ch <- saveTreeJob{
		snPath: snPath,
		node:   node,
		nodes:  nodes,
		ch:     ch,
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
	err   error
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

func (s *TreeSaver) worker(ctx context.Context, wg *sync.WaitGroup, jobs <-chan saveTreeJob) {
	defer wg.Done()
	for {
		var job saveTreeJob
		select {
		case <-ctx.Done():
			return
		case job = <-jobs:
		}

		node, stats, err := s.save(ctx, job.snPath, job.node, job.nodes)
		job.ch <- saveTreeResponse{
			node:  node,
			stats: stats,
			err:   err,
		}
		close(job.ch)
	}
}
