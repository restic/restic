package checker

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/progress"
	"golang.org/x/sync/errgroup"
)

// Checker runs various checks on a repository. It is advisable to create an
// exclusive Lock in the repository before running any checks.
//
// A Checker only tests for internal errors within the data structures of the
// repository (e.g. missing blobs), and needs a valid Repository to work on.
type Checker struct {
	*repository.Checker
	blobRefs struct {
		sync.Mutex
		M restic.AssociatedBlobSet
	}
	trackUnused bool

	snapshots restic.Lister

	repo restic.Repository
}

type checkerRepository interface {
	restic.Repository
	Checker() *repository.Checker
}

// New returns a new checker which runs on repo.
func New(repo checkerRepository, trackUnused bool) *Checker {
	c := &Checker{
		Checker:     repo.Checker(),
		repo:        repo,
		trackUnused: trackUnused,
	}

	c.blobRefs.M = c.repo.NewAssociatedBlobSet()

	return c
}

func (c *Checker) LoadSnapshots(ctx context.Context) error {
	var err error
	c.snapshots, err = restic.MemorizeList(ctx, c.repo, restic.SnapshotFile)
	return err
}

// Error is an error that occurred while checking a repository.
type Error struct {
	TreeID restic.ID
	Err    error
}

func (e *Error) Error() string {
	if !e.TreeID.IsNull() {
		return "tree " + e.TreeID.String() + ": " + e.Err.Error()
	}

	return e.Err.Error()
}

// TreeError collects several errors that occurred while processing a tree.
type TreeError struct {
	ID     restic.ID
	Errors []error
}

func (e *TreeError) Error() string {
	return fmt.Sprintf("tree %v: %v", e.ID, e.Errors)
}

// checkTreeWorker checks the trees received and sends out errors to errChan.
func (c *Checker) checkTreeWorker(ctx context.Context, trees <-chan data.TreeItem, out chan<- error) {
	for job := range trees {
		debug.Log("check tree %v (tree %v, err %v)", job.ID, job.Tree, job.Error)

		var errs []error
		if job.Error != nil {
			errs = append(errs, job.Error)
		} else {
			errs = c.checkTree(job.ID, job.Tree)
		}

		if len(errs) == 0 {
			continue
		}
		treeError := &TreeError{ID: job.ID, Errors: errs}
		select {
		case <-ctx.Done():
			return
		case out <- treeError:
			debug.Log("tree %v: sent %d errors", treeError.ID, len(treeError.Errors))
		}
	}
}

func loadSnapshotTreeIDs(ctx context.Context, lister restic.Lister, repo restic.LoaderUnpacked) (ids restic.IDs, errs []error) {
	err := data.ForAllSnapshots(ctx, lister, repo, nil, func(id restic.ID, sn *data.Snapshot, err error) error {
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		treeID := *sn.Tree
		debug.Log("snapshot %v has tree %v", id, treeID)
		ids = append(ids, treeID)
		return nil
	})
	if err != nil {
		errs = append(errs, err)
	}

	return ids, errs
}

// Structure checks that for all snapshots all referenced data blobs and
// subtrees are available in the index. errChan is closed after all trees have
// been traversed.
func (c *Checker) Structure(ctx context.Context, p *progress.Counter, errChan chan<- error) {
	trees, errs := loadSnapshotTreeIDs(ctx, c.snapshots, c.repo)
	p.SetMax(uint64(len(trees)))
	debug.Log("need to check %d trees from snapshots, %d errs returned", len(trees), len(errs))

	for _, err := range errs {
		select {
		case <-ctx.Done():
			return
		case errChan <- err:
		}
	}

	wg, ctx := errgroup.WithContext(ctx)
	treeStream := data.StreamTrees(ctx, wg, c.repo, trees, func(treeID restic.ID) bool {
		// blobRefs may be accessed in parallel by checkTree
		c.blobRefs.Lock()
		h := restic.BlobHandle{ID: treeID, Type: restic.TreeBlob}
		blobReferenced := c.blobRefs.M.Has(h)
		// noop if already referenced
		c.blobRefs.M.Insert(h)
		c.blobRefs.Unlock()
		return blobReferenced
	}, p)

	defer close(errChan)
	// The checkTree worker only processes already decoded trees and is thus CPU-bound
	workerCount := runtime.GOMAXPROCS(0)
	for i := 0; i < workerCount; i++ {
		wg.Go(func() error {
			c.checkTreeWorker(ctx, treeStream, errChan)
			return nil
		})
	}

	// the wait group should not return an error because no worker returns an
	// error, so panic if that has changed somehow.
	err := wg.Wait()
	if err != nil {
		panic(err)
	}
}

func (c *Checker) checkTree(id restic.ID, tree *data.Tree) (errs []error) {
	debug.Log("checking tree %v", id)

	for _, node := range tree.Nodes {
		switch node.Type {
		case data.NodeTypeFile:
			if node.Content == nil {
				errs = append(errs, &Error{TreeID: id, Err: errors.Errorf("file %q has nil blob list", node.Name)})
			}

			for b, blobID := range node.Content {
				if blobID.IsNull() {
					errs = append(errs, &Error{TreeID: id, Err: errors.Errorf("file %q blob %d has null ID", node.Name, b)})
					continue
				}
				// Note that we do not use the blob size. The "obvious" check
				// whether the sum of the blob sizes matches the file size
				// unfortunately fails in some cases that are not resolvable
				// by users, so we omit this check, see #1887

				_, found := c.repo.LookupBlobSize(restic.DataBlob, blobID)
				if !found {
					debug.Log("tree %v references blob %v which isn't contained in index", id, blobID)
					errs = append(errs, &Error{TreeID: id, Err: errors.Errorf("file %q blob %v not found in index", node.Name, blobID)})
				}
			}

			if c.trackUnused {
				// loop a second time to keep the locked section as short as possible
				c.blobRefs.Lock()
				for _, blobID := range node.Content {
					if blobID.IsNull() {
						continue
					}
					h := restic.BlobHandle{ID: blobID, Type: restic.DataBlob}
					c.blobRefs.M.Insert(h)
					debug.Log("blob %v is referenced", blobID)
				}
				c.blobRefs.Unlock()
			}

		case data.NodeTypeDir:
			if node.Subtree == nil {
				errs = append(errs, &Error{TreeID: id, Err: errors.Errorf("dir node %q has no subtree", node.Name)})
				continue
			}

			if node.Subtree.IsNull() {
				errs = append(errs, &Error{TreeID: id, Err: errors.Errorf("dir node %q subtree id is null", node.Name)})
				continue
			}

		case data.NodeTypeSymlink, data.NodeTypeSocket, data.NodeTypeCharDev, data.NodeTypeDev, data.NodeTypeFifo:
			// nothing to check

		default:
			errs = append(errs, &Error{TreeID: id, Err: errors.Errorf("node %q with invalid type %q", node.Name, node.Type)})
		}

		if node.Name == "" {
			errs = append(errs, &Error{TreeID: id, Err: errors.New("node with empty name")})
		}
	}

	return errs
}

// UnusedBlobs returns all blobs that have never been referenced.
func (c *Checker) UnusedBlobs(ctx context.Context) (blobs restic.BlobHandles, err error) {
	if !c.trackUnused {
		panic("only works when tracking blob references")
	}
	c.blobRefs.Lock()
	defer c.blobRefs.Unlock()

	debug.Log("checking %d blobs", c.blobRefs.M.Len())
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	err = c.repo.ListBlobs(ctx, func(blob restic.PackedBlob) {
		h := restic.BlobHandle{ID: blob.ID, Type: blob.Type}
		if !c.blobRefs.M.Has(h) {
			debug.Log("blob %v not referenced", h)
			blobs = append(blobs, h)
		}
	})

	return blobs, err
}
