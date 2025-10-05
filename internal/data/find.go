package data

import (
	"context"
	"sync"

	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/progress"
	"golang.org/x/sync/errgroup"
)

// FindUsedBlobs traverses the tree ID and adds all seen blobs (trees and data
// blobs) to the set blobs. Already seen tree blobs will not be visited again.
func FindUsedBlobs(ctx context.Context, repo restic.Loader, treeIDs restic.IDs, blobs restic.FindBlobSet, p *progress.Counter) error {
	var lock sync.Mutex

	wg, ctx := errgroup.WithContext(ctx)
	treeStream := StreamTrees(ctx, wg, repo, treeIDs, func(treeID restic.ID) bool {
		// locking is necessary the goroutine below concurrently adds data blobs
		lock.Lock()
		h := restic.BlobHandle{ID: treeID, Type: restic.TreeBlob}
		blobReferenced := blobs.Has(h)
		// noop if already referenced
		blobs.Insert(h)
		lock.Unlock()
		return blobReferenced
	}, p)

	wg.Go(func() error {
		for tree := range treeStream {
			if tree.Error != nil {
				return tree.Error
			}

			lock.Lock()
			for _, node := range tree.Nodes {
				switch node.Type {
				case NodeTypeFile:
					for _, blob := range node.Content {
						blobs.Insert(restic.BlobHandle{ID: blob, Type: restic.DataBlob})
					}
				}
			}
			lock.Unlock()
		}
		return nil
	})
	return wg.Wait()
}
