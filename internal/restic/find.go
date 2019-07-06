package restic

import (
	"context"
	"runtime"
	"sync"

	tomb "gopkg.in/tomb.v2"
)

// FindUsedBlobs traverses the tree IDs and returns all seen blobs (trees and
// data blobs).
func FindUsedBlobs(ctx context.Context, repo Repository, snapshots []*Snapshot, p *Progress) (BlobSet, error) {
	p.Start()
	defer p.Done()

	blobs := NewBlobSet()
	var seen sync.Map

	t, wctx := tomb.WithContext(ctx)

	inputCh := make(chan ID)
	blobCh := make(chan BlobHandle)

	// Feed the tree IDs to the worker goroutines
	inputWorker := func() error {
		for _, sn := range snapshots {
			select {
			case inputCh <- *sn.Tree:
			case <-t.Dying():
				close(inputCh)
				return tomb.ErrDying
			}
		}
		close(inputCh)
		return nil
	}

	// Collect the found blob IDs
	collectWorker := func() error {
		for h := range blobCh {
			blobs.Insert(h)
		}
		return nil
	}

	// Recursively scan a tree, skipping already seen sub-trees
	var scanner func(treeID ID) error
	scanner = func(treeID ID) error {
		blobCh <- BlobHandle{ID: treeID, Type: TreeBlob}

		tree, err := repo.LoadTree(wctx, treeID)
		if err != nil {
			return err
		}

		for _, node := range tree.Nodes {
			switch node.Type {
			case "file":
				for _, blob := range node.Content {
					select {
					case blobCh <- BlobHandle{ID: blob, Type: DataBlob}:
					case <-t.Dying():
						return tomb.ErrDying
					}
				}
			case "dir":
				subtreeID := *node.Subtree
				h := BlobHandle{ID: subtreeID, Type: TreeBlob}
				if _, ok := seen.LoadOrStore(h, nil); ok {
					continue
				}

				err := scanner(subtreeID)
				if err != nil {
					return err
				}
			}
		}

		return nil
	}

	// Process the snapshots
	var workerWG sync.WaitGroup
	worker := func() error {
		defer workerWG.Done()
		for treeID := range inputCh {
			err := scanner(treeID)
			if err != nil {
				return err
			}
			p.Report(Stat{Blobs: 1})

			select {
			case <-t.Dying():
				return tomb.ErrDying
			default:
			}
		}
		return nil
	}

	t.Go(func() error {
		t.Go(inputWorker)
		t.Go(collectWorker)

		workerCount := runtime.GOMAXPROCS(0)
		workerWG.Add(workerCount)
		for i := 0; i < workerCount; i++ {
			t.Go(worker)
		}
		t.Go(func() error {
			workerWG.Wait()
			close(blobCh)
			return nil
		})
		return nil
	})

	err := t.Wait()
	if err != nil {
		return nil, err
	}

	return blobs, nil
}
