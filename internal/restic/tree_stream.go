package restic

import (
	"context"
	"errors"
	"runtime"
	"sync"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/ui/progress"
	"golang.org/x/sync/errgroup"
)

// TreeItem is used to return either an error or the tree for a tree id
type TreeItem struct {
	ID
	Error error
	*Tree
}

type trackedTreeItem struct {
	TreeItem
	rootIdx int
}

type trackedID struct {
	ID
	rootIdx int
}

// loadTreeWorker loads trees from repo and sends them to out.
func loadTreeWorker(ctx context.Context, repo Loader,
	in <-chan trackedID, out chan<- trackedTreeItem) {

	for treeID := range in {
		tree, err := LoadTree(ctx, repo, treeID.ID)
		debug.Log("load tree %v (%v) returned err: %v", tree, treeID, err)
		job := trackedTreeItem{TreeItem: TreeItem{ID: treeID.ID, Error: err, Tree: tree}, rootIdx: treeID.rootIdx}

		select {
		case <-ctx.Done():
			return
		case out <- job:
		}
	}
}

func filterTrees(ctx context.Context, repo Loader, trees IDs, loaderChan chan<- trackedID, hugeTreeLoaderChan chan<- trackedID,
	in <-chan trackedTreeItem, out chan<- TreeItem, skip func(tree ID) bool, p *progress.Counter) {

	var (
		inCh                    = in
		outCh                   chan<- TreeItem
		loadCh                  chan<- trackedID
		job                     TreeItem
		nextTreeID              trackedID
		outstandingLoadTreeJobs = 0
	)
	rootCounter := make([]int, len(trees))
	backlog := make([]trackedID, 0, len(trees))
	for idx, id := range trees {
		backlog = append(backlog, trackedID{ID: id, rootIdx: idx})
		rootCounter[idx] = 1
	}

	for {
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

			treeSize, found := repo.LookupBlobSize(TreeBlob, nextTreeID.ID)
			if found && treeSize > 50*1024*1024 {
				loadCh = hugeTreeLoaderChan
			} else {
				loadCh = loaderChan
			}
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
			rootCounter[j.rootIdx]--

			debug.Log("input job tree %v", j.ID)

			if j.Error != nil {
				debug.Log("received job with error: %v (tree %v, ID %v)", j.Error, j.Tree, j.ID)
			} else if j.Tree == nil {
				debug.Log("received job with nil tree pointer: %v (ID %v)", j.Error, j.ID)
				// send a new job with the new error instead of the old one
				j = trackedTreeItem{TreeItem: TreeItem{ID: j.ID, Error: errors.New("tree is nil and error is nil")}, rootIdx: j.rootIdx}
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
					backlog = append(backlog, trackedID{ID: id, rootIdx: j.rootIdx})
					rootCounter[j.rootIdx]++
				}
			}
			if p != nil && rootCounter[j.rootIdx] == 0 {
				p.Add(1)
			}

			job = j.TreeItem
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
// is guaranteed to always be called from the same goroutine. To shutdown the started
// goroutines, either read all items from the channel or cancel the context. Then `Wait()`
// on the errgroup until all goroutines were stopped.
func StreamTrees(ctx context.Context, wg *errgroup.Group, repo Loader, trees IDs, skip func(tree ID) bool, p *progress.Counter) <-chan TreeItem {
	loaderChan := make(chan trackedID)
	hugeTreeChan := make(chan trackedID, 10)
	loadedTreeChan := make(chan trackedTreeItem)
	treeStream := make(chan TreeItem)

	var loadTreeWg sync.WaitGroup

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
			loadTreeWorker(ctx, repo, workerLoaderChan, loadedTreeChan)
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
		defer close(hugeTreeChan)
		defer close(treeStream)
		filterTrees(ctx, repo, trees, loaderChan, hugeTreeChan, loadedTreeChan, treeStream, skip, p)
		return nil
	})
	return treeStream
}
