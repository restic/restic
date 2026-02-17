package data

import (
	"context"
	"runtime"
	"sync"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/progress"
	"golang.org/x/sync/errgroup"
)

type trackedTreeItem struct {
	restic.ID
	Subtrees restic.IDs
	rootIdx  int
}

type trackedID struct {
	restic.ID
	rootIdx int
}

// subtreesCollector wraps a TreeNodeIterator and returns a new iterator that collects the subtrees.
func subtreesCollector(tree TreeNodeIterator) (TreeNodeIterator, func() restic.IDs) {
	subtrees := restic.IDs{}
	isComplete := false

	return func(yield func(NodeOrError) bool) {
			for item := range tree {
				if !yield(item) {
					return
				}
				// be defensive and check for nil subtree as this code is also used by the checker
				if item.Node != nil && item.Node.Type == NodeTypeDir && item.Node.Subtree != nil {
					subtrees = append(subtrees, *item.Node.Subtree)
				}
			}
			isComplete = true
		}, func() restic.IDs {
			if !isComplete {
				panic("tree was not read completely")
			}
			return subtrees
		}
}

// loadTreeWorker loads trees from repo and sends them to out.
func loadTreeWorker(
	ctx context.Context,
	repo restic.Loader,
	in <-chan trackedID,
	process func(id restic.ID, error error, nodes TreeNodeIterator) error,
	out chan<- trackedTreeItem,
) error {

	for treeID := range in {
		tree, err := LoadTree(ctx, repo, treeID.ID)
		if tree == nil && err == nil {
			err = errors.New("tree is nil and error is nil")
		}
		debug.Log("load tree %v (%v) returned err: %v", tree, treeID, err)

		//  wrap iterator to collect subtrees while `process` iterates over `tree`
		var collectSubtrees func() restic.IDs
		if tree != nil {
			tree, collectSubtrees = subtreesCollector(tree)
		}

		err = process(treeID.ID, err, tree)
		if err != nil {
			return err
		}

		// assume that the number of subtrees is within reasonable limits, such that the memory usage is not a problem
		var subtrees restic.IDs
		if collectSubtrees != nil {
			subtrees = collectSubtrees()
		}

		job := trackedTreeItem{ID: treeID.ID, Subtrees: subtrees, rootIdx: treeID.rootIdx}

		select {
		case <-ctx.Done():
			return nil
		case out <- job:
		}
	}
	return nil
}

// filterTree receives the result of a tree load and queues new trees for loading and processing.
func filterTrees(ctx context.Context, repo restic.Loader, trees restic.IDs, loaderChan chan<- trackedID, hugeTreeLoaderChan chan<- trackedID,
	in <-chan trackedTreeItem, skip func(tree restic.ID) bool, p *progress.Counter) {

	var (
		inCh                    = in
		loadCh                  chan<- trackedID
		nextTreeID              trackedID
		outstandingLoadTreeJobs = 0
	)
	// tracks how many trees are currently waiting to be processed for a given root tree
	rootCounter := make([]int, len(trees))
	// build initial backlog
	backlog := make([]trackedID, 0, len(trees))
	for idx, id := range trees {
		backlog = append(backlog, trackedID{ID: id, rootIdx: idx})
		rootCounter[idx] = 1
	}

	for {
		// if no tree is waiting to be sent, pick the next one
		if loadCh == nil && len(backlog) > 0 {
			// process last added ids first, that is traverse the tree in depth-first order
			ln := len(backlog) - 1
			nextTreeID, backlog = backlog[ln], backlog[:ln]

			if skip(nextTreeID.ID) {
				rootCounter[nextTreeID.rootIdx]--
				if p != nil && rootCounter[nextTreeID.rootIdx] == 0 {
					p.Add(1)
				}
				continue
			}

			treeSize, found := repo.LookupBlobSize(restic.TreeBlob, nextTreeID.ID)
			if found && treeSize > 50*1024*1024 {
				loadCh = hugeTreeLoaderChan
			} else {
				loadCh = loaderChan
			}
		}

		// loadCh is only nil at this point if the backlog is empty
		if loadCh == nil && outstandingLoadTreeJobs == 0 {
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
				continue
			}

			outstandingLoadTreeJobs--
			rootCounter[j.rootIdx]--

			debug.Log("input job tree %v", j.ID)
			// iterate backwards over subtree to compensate backwards traversal order of nextTreeID selection
			for i := len(j.Subtrees) - 1; i >= 0; i-- {
				id := j.Subtrees[i]
				if id.IsNull() {
					// We do not need to raise this error here, it is
					// checked when the tree is checked. Just make sure
					// that we do not add any null IDs to the backlog.
					debug.Log("tree %v has nil subtree", j.ID)
					continue
				}
				backlog = append(backlog, trackedID{ID: id, rootIdx: j.rootIdx})
				rootCounter[j.rootIdx]++
			}
			// the progress check must happen after j.Subtrees was added to the backlog
			if p != nil && rootCounter[j.rootIdx] == 0 {
				p.Add(1)
			}
		}
	}
}

// StreamTrees iteratively loads the given trees and their subtrees. The skip method
// is guaranteed to always be called from the same goroutine. The process function is
// directly called from the worker goroutines. It MUST read `nodes` until it returns an
// error or completes. If the process function returns an error, then StreamTrees will
// abort and return the error.
func StreamTrees(
	ctx context.Context,
	repo restic.Loader,
	trees restic.IDs,
	p *progress.Counter,
	skip func(tree restic.ID) bool,
	process func(id restic.ID, error error, nodes TreeNodeIterator) error,
) error {
	loaderChan := make(chan trackedID)
	hugeTreeChan := make(chan trackedID, 10)
	loadedTreeChan := make(chan trackedTreeItem)

	var loadTreeWg sync.WaitGroup

	wg, ctx := errgroup.WithContext(ctx)
	// decoding a tree can take quite some time such that this can be both CPU- or IO-bound
	// one extra worker to handle huge tree blobs
	workerCount := int(repo.Connections()) + runtime.GOMAXPROCS(0) + 1
	for i := 0; i < workerCount; i++ {
		workerLoaderChan := loaderChan
		if i == 0 {
			workerLoaderChan = hugeTreeChan
		}
		loadTreeWg.Add(1)
		wg.Go(func() error {
			defer loadTreeWg.Done()
			return loadTreeWorker(ctx, repo, workerLoaderChan, process, loadedTreeChan)
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
		defer close(hugeTreeChan)
		filterTrees(ctx, repo, trees, loaderChan, hugeTreeChan, loadedTreeChan, skip, p)
		return nil
	})
	return wg.Wait()
}
