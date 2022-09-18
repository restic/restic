package repository

import (
	"context"
	"sync"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/progress"

	"golang.org/x/sync/errgroup"
)

// Repack takes a list of packs together with a list of blobs contained in
// these packs. Each pack is loaded and the blobs listed in keepBlobs is saved
// into a new pack. Returned is the list of obsolete packs which can then
// be removed.
//
// The map keepBlobs is modified by Repack, it is used to keep track of which
// blobs have been processed.
func Repack(ctx context.Context, repo restic.Repository, dstRepo restic.Repository, packs restic.IDSet, keepBlobs restic.BlobSet, p *progress.Counter) (obsoletePacks restic.IDSet, err error) {
	debug.Log("repacking %d packs while keeping %d blobs", len(packs), len(keepBlobs))

	if repo == dstRepo && dstRepo.Connections() < 2 {
		return nil, errors.Fatal("repack step requires a backend connection limit of at least two")
	}

	wg, wgCtx := errgroup.WithContext(ctx)

	dstRepo.StartPackUploader(wgCtx, wg)
	wg.Go(func() error {
		var err error
		obsoletePacks, err = repack(wgCtx, repo, dstRepo, packs, keepBlobs, p)
		return err
	})

	if err := wg.Wait(); err != nil {
		return nil, err
	}
	return obsoletePacks, nil
}

func repack(ctx context.Context, repo restic.Repository, dstRepo restic.Repository, packs restic.IDSet, keepBlobs restic.BlobSet, p *progress.Counter) (obsoletePacks restic.IDSet, err error) {
	wg, wgCtx := errgroup.WithContext(ctx)

	var keepMutex sync.Mutex
	downloadQueue := make(chan restic.PackBlobs)
	wg.Go(func() error {
		defer close(downloadQueue)
		for pbs := range repo.Index().ListPacks(wgCtx, packs) {
			var packBlobs []restic.Blob
			keepMutex.Lock()
			// filter out unnecessary blobs
			for _, entry := range pbs.Blobs {
				h := restic.BlobHandle{ID: entry.ID, Type: entry.Type}
				if keepBlobs.Has(h) {
					packBlobs = append(packBlobs, entry)
				}
			}
			keepMutex.Unlock()

			select {
			case downloadQueue <- restic.PackBlobs{PackID: pbs.PackID, Blobs: packBlobs}:
			case <-wgCtx.Done():
				return wgCtx.Err()
			}
		}
		return nil
	})

	worker := func() error {
		for t := range downloadQueue {
			err := StreamPack(wgCtx, repo.Backend().Load, repo.Key(), t.PackID, t.Blobs, func(blob restic.BlobHandle, buf []byte, err error) error {
				if err != nil {
					var ierr error
					// check whether we can get a valid copy somewhere else
					buf, ierr = repo.LoadBlob(wgCtx, blob.Type, blob.ID, nil)
					if ierr != nil {
						// no luck, return the original error
						return err
					}
				}

				keepMutex.Lock()
				// recheck whether some other worker was faster
				shouldKeep := keepBlobs.Has(blob)
				if shouldKeep {
					keepBlobs.Delete(blob)
				}
				keepMutex.Unlock()

				if !shouldKeep {
					return nil
				}

				// We do want to save already saved blobs!
				_, _, _, err = dstRepo.SaveBlob(wgCtx, blob.Type, buf, blob.ID, true)
				if err != nil {
					return err
				}

				debug.Log("  saved blob %v", blob.ID)
				return nil
			})
			if err != nil {
				return err
			}
			p.Add(1)
		}
		return nil
	}

	// as packs are streamed the concurrency is limited by IO
	// reduce by one to ensure that uploading is always possible
	repackWorkerCount := int(repo.Connections() - 1)
	if repo != dstRepo {
		// no need to share the upload and download connections for different repositories
		repackWorkerCount = int(repo.Connections())
	}
	for i := 0; i < repackWorkerCount; i++ {
		wg.Go(worker)
	}

	if err := wg.Wait(); err != nil {
		return nil, err
	}

	if err := dstRepo.Flush(ctx); err != nil {
		return nil, err
	}

	return packs, nil
}
