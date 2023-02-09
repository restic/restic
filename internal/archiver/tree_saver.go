package archiver

import (
	"context"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/sync/errgroup"
)

// TreeSaver concurrently saves incoming trees to the repo.
type TreeSaver struct {
	saveTree func(context.Context, *restic.TreeJSONBuilder) (restic.ID, ItemStats, error)
	errFn    ErrorFunc

	ch chan<- saveTreeJob
}

// NewTreeSaver returns a new tree saver. A worker pool with treeWorkers is
// started, it is stopped when ctx is cancelled.
func NewTreeSaver(ctx context.Context, wg *errgroup.Group, treeWorkers uint, saveTree func(context.Context, *restic.TreeJSONBuilder) (restic.ID, ItemStats, error), errFn ErrorFunc) *TreeSaver {
	ch := make(chan saveTreeJob)

	s := &TreeSaver{
		ch:       ch,
		saveTree: saveTree,
		errFn:    errFn,
	}

	for i := uint(0); i < treeWorkers; i++ {
		wg.Go(func() error {
			return s.worker(ctx, ch)
		})
	}

	return s
}

func (s *TreeSaver) TriggerShutdown() {
	close(s.ch)
}

// Save stores the dir d and returns the data once it has been completed.
func (s *TreeSaver) Save(ctx context.Context, snPath string, target string, node *restic.Node, nodes []FutureNode, complete CompleteFunc) FutureNode {
	fn, ch := newFutureNode()
	job := saveTreeJob{
		snPath:   snPath,
		target:   target,
		node:     node,
		nodes:    nodes,
		ch:       ch,
		complete: complete,
	}
	select {
	case s.ch <- job:
	case <-ctx.Done():
		debug.Log("not saving tree, context is cancelled")
		close(ch)
	}

	return fn
}

type saveTreeJob struct {
	snPath   string
	target   string
	node     *restic.Node
	nodes    []FutureNode
	ch       chan<- futureNodeResult
	complete CompleteFunc
}

// save stores the nodes as a tree in the repo.
func (s *TreeSaver) save(ctx context.Context, job *saveTreeJob) (*restic.Node, ItemStats, error) {
	var stats ItemStats
	node := job.node
	nodes := job.nodes
	// allow GC of nodes array once the loop is finished
	job.nodes = nil

	builder := restic.NewTreeJSONBuilder()

	for i, fn := range nodes {
		// fn is a copy, so clear the original value explicitly
		nodes[i] = FutureNode{}
		fnr := fn.take(ctx)

		// return the error if it wasn't ignored
		if fnr.err != nil {
			debug.Log("err for %v: %v", fnr.snPath, fnr.err)
			fnr.err = s.errFn(fnr.target, fnr.err)
			if fnr.err == nil {
				// ignore error
				continue
			}

			return nil, stats, fnr.err
		}

		// when the error is ignored, the node could not be saved, so ignore it
		if fnr.node == nil {
			debug.Log("%v excluded: %v", fnr.snPath, fnr.target)
			continue
		}

		debug.Log("insert %v", fnr.node.Name)
		err := builder.AddNode(fnr.node)
		if err != nil {
			return nil, stats, err
		}
	}

	id, treeStats, err := s.saveTree(ctx, builder)
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
		var ok bool
		select {
		case <-ctx.Done():
			return nil
		case job, ok = <-jobs:
			if !ok {
				return nil
			}
		}

		node, stats, err := s.save(ctx, &job)
		if err != nil {
			debug.Log("error saving tree blob: %v", err)
			close(job.ch)
			return err
		}

		if job.complete != nil {
			job.complete(node, stats)
		}
		job.ch <- futureNodeResult{
			snPath: job.snPath,
			target: job.target,
			node:   node,
			stats:  stats,
		}
		close(job.ch)
	}
}
