package archiver

import (
	"context"
	"errors"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/sync/errgroup"
)

// treeSaver concurrently saves incoming trees to the repo.
type treeSaver struct {
	uploader restic.BlobSaverAsync
	errFn    ErrorFunc

	ch chan<- saveTreeJob
}

// newTreeSaver returns a new tree saver. A worker pool with treeWorkers is
// started, it is stopped when ctx is cancelled.
func newTreeSaver(ctx context.Context, wg *errgroup.Group, treeWorkers uint, uploader restic.BlobSaverAsync, errFn ErrorFunc) *treeSaver {
	ch := make(chan saveTreeJob)

	s := &treeSaver{
		ch:       ch,
		uploader: uploader,
		errFn:    errFn,
	}

	for range treeWorkers {
		wg.Go(func() error {
			return s.worker(ctx, ch)
		})
	}

	return s
}

func (s *treeSaver) TriggerShutdown() {
	close(s.ch)
}

// Save stores the dir d and returns the data once it has been completed.
func (s *treeSaver) Save(ctx context.Context, snPath string, target string, node *data.Node, builder *treeBuilder, nodes []futureNode, complete fileCompleteFunc) futureNode {
	fn, ch := newFutureNode()
	job := saveTreeJob{
		snPath:   snPath,
		target:   target,
		node:     node,
		builder:  builder,
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
	snPath string
	target string
	node   *data.Node
	// builder holds the entries folded in already, for a directory too wide to
	// keep all of its entries in memory. nil means none have been folded yet.
	builder  *treeBuilder
	nodes    []futureNode
	ch       chan<- futureNodeResult
	complete fileCompleteFunc
}

// treeBuilder folds the completed entries of one directory into a tree blob.
// Entries have to arrive in the order they appear in the directory, so an entry
// that is not finished yet holds up the ones behind it.
type treeBuilder struct {
	builder  *data.TreeJSONBuilder
	lastNode *data.Node
	errFn    ErrorFunc
}

func newTreeBuilder(errFn ErrorFunc, expectedEntries int) *treeBuilder {
	return &treeBuilder{
		builder: data.NewTreeJSONBuilderForEntries(expectedEntries),
		errFn:   errFn,
	}
}

// add appends the result for one directory entry. A returned error aborts the
// backup; an error the error handler chooses to ignore skips the entry instead.
func (tb *treeBuilder) add(fnr futureNodeResult) error {
	// return the error if it wasn't ignored
	if fnr.err != nil {
		debug.Log("err for %v: %v", fnr.snPath, fnr.err)
		if fnr.err == context.Canceled {
			return fnr.err
		}

		fnr.err = tb.errFn(fnr.target, fnr.err)
		if fnr.err == nil {
			// ignore error
			return nil
		}

		return fnr.err
	}

	// when the error is ignored, the node could not be saved, so ignore it
	if fnr.node == nil {
		debug.Log("%v excluded: %v", fnr.snPath, fnr.target)
		return nil
	}

	err := tb.builder.AddNode(fnr.node)
	if err != nil && errors.Is(err, data.ErrTreeNotOrdered) && tb.lastNode != nil && fnr.node.Equals(*tb.lastNode) {
		debug.Log("insert %v failed: %v", fnr.node.Name, err)
		// ignore error if an _identical_ node already exists, but nevertheless issue a warning
		_ = tb.errFn(fnr.target, err)
		err = nil
	}
	if err != nil {
		debug.Log("insert %v failed: %v", fnr.node.Name, err)
		return err
	}
	tb.lastNode = fnr.node
	return nil
}

// save stores the nodes as a tree in the repo.
func (s *treeSaver) save(ctx context.Context, job *saveTreeJob) (*data.Node, ItemStats, error) {
	var stats ItemStats
	node := job.node
	nodes := job.nodes
	// allow GC of nodes array once the loop is finished
	job.nodes = nil

	tb := job.builder
	if tb == nil {
		tb = newTreeBuilder(s.errFn, len(nodes))
	}
	job.builder = nil

	for i, fn := range nodes {
		// fn is a copy, so clear the original value explicitly
		nodes[i] = futureNode{}
		if err := tb.add(fn.take(ctx)); err != nil {
			return nil, stats, err
		}
	}

	buf, err := tb.builder.Finalize()
	if err != nil {
		return nil, stats, err
	}

	var (
		known      bool
		length     int
		sizeInRepo int
		id         restic.ID
	)

	ch := make(chan struct{}, 1)
	s.uploader.SaveBlobAsync(ctx, restic.TreeBlob, buf, restic.ID{}, false, func(newID restic.ID, cbKnown bool, cbSizeInRepo int, cbErr error) {
		known = cbKnown
		length = len(buf)
		sizeInRepo = cbSizeInRepo
		id = newID
		err = cbErr
		ch <- struct{}{}
	})

	select {
	case <-ch:
		if err != nil {
			return nil, stats, err
		}
		if !known {
			stats.TreeBlobs++
			stats.TreeSize += uint64(length)
			stats.TreeSizeInRepo += uint64(sizeInRepo)
		}

		node.Subtree = &id
		return node, stats, nil
	case <-ctx.Done():
		return nil, stats, ctx.Err()
	}
}

func (s *treeSaver) worker(ctx context.Context, jobs <-chan saveTreeJob) error {
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
