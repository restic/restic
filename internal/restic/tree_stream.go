package restic

import (
	"context"
	"errors"
	"sync"

	"github.com/restic/restic/internal/debug"
	"golang.org/x/sync/errgroup"
)

const streamTreeParallelism = 5

// TreeItem is used to return either an error or the tree for a tree id
type TreeItem struct {
	ID
	Error error
	*Tree
}

// loadTreeWorker loads trees from repo and sends them to out.
func loadTreeWorker(ctx context.Context, repo TreeLoader,
	in <-chan ID, out chan<- TreeItem) {

	for treeID := range in {
		tree, err := repo.LoadTree(ctx, treeID)
		debug.Log("load tree %v (%v) returned err: %v", tree, treeID, err)
		job := TreeItem{ID: treeID, Error: err, Tree: tree}

		select {
		case <-ctx.Done():
			return
		case out <- job:
		}
	}
}

func filterTrees(ctx context.Context, backlog IDs, loaderChan chan<- ID,
	in <-chan TreeItem, out chan<- TreeItem, skip func(tree ID) bool) {

	var (
		inCh                    = in
		outCh                   chan<- TreeItem
		loadCh                  chan<- ID
		job                     TreeItem
		nextTreeID              ID
		outstandingLoadTreeJobs = 0
	)

	for {
		if loadCh == nil && len(backlog) > 0 {
			// process last added ids first, that is traverse the tree in depth-first order
			ln := len(backlog) - 1
			nextTreeID, backlog = backlog[ln], backlog[:ln]

			if skip(nextTreeID) {
				continue
			}

			loadCh = loaderChan
		}

		if loadCh == nil && outCh == nil && outstandingLoadTreeJobs == 0 {
			debug.Log("backlog is empty, all channels nil, exiting")
			return
		}

		select {
		case <-ctx.Done():
			return

		case loadCh <- nextTreeID:
			outstandingLoadTreeJobs++
			loadCh = nil

		case j, ok := <-inCh:
			if !ok {
				debug.Log("input channel closed")
				inCh = nil
				in = nil
				continue
			}

			outstandingLoadTreeJobs--

			debug.Log("input job tree %v", j.ID)

			if j.Error != nil {
				debug.Log("received job with error: %v (tree %v, ID %v)", j.Error, j.Tree, j.ID)
			} else if j.Tree == nil {
				debug.Log("received job with nil tree pointer: %v (ID %v)", j.Error, j.ID)
				// send a new job with the new error instead of the old one
				j = TreeItem{ID: j.ID, Error: errors.New("tree is nil and error is nil")}
			} else {
				subtrees := j.Tree.Subtrees()
				debug.Log("subtrees for tree %v: %v", j.ID, subtrees)
				// iterate backwards over subtree to compensate backwards traversal order of nextTreeID selection
				for i := len(subtrees) - 1; i >= 0; i-- {
					id := subtrees[i]
					if id.IsNull() {
						// We do not need to raise this error here, it is
						// checked when the tree is checked. Just make sure
						// that we do not add any null IDs to the backlog.
						debug.Log("tree %v has nil subtree", j.ID)
						continue
					}
					backlog = append(backlog, id)
				}
			}

			job = j
			outCh = out
			inCh = nil

		case outCh <- job:
			debug.Log("tree sent to process: %v", job.ID)
			outCh = nil
			inCh = in
		}
	}
}

// StreamTrees iteratively loads the given trees and their subtrees. The skip method
// is guaranteed to always be called from the same goroutine.
func StreamTrees(ctx context.Context, wg *errgroup.Group, repo TreeLoader, trees IDs, skip func(tree ID) bool) <-chan TreeItem {
	loaderChan := make(chan ID)
	loadedTreeChan := make(chan TreeItem)
	treeStream := make(chan TreeItem)

	var loadTreeWg sync.WaitGroup

	for i := 0; i < streamTreeParallelism; i++ {
		loadTreeWg.Add(1)
		wg.Go(func() error {
			defer loadTreeWg.Done()
			loadTreeWorker(ctx, repo, loaderChan, loadedTreeChan)
			return nil
		})
	}

	// close once all loadTreeWorkers have completed
	wg.Go(func() error {
		loadTreeWg.Wait()
		close(loadedTreeChan)
		return nil
	})

	wg.Go(func() error {
		defer close(loaderChan)
		defer close(treeStream)
		filterTrees(ctx, trees, loaderChan, loadedTreeChan, treeStream, skip)
		return nil
	})
	return treeStream
}
