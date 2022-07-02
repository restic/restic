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

const numRepackWorkers = 8

// Repack takes a list of packs together with a list of blobs contained in
// these packs. Each pack is loaded and the blobs listed in keepBlobs is saved
// into a new pack. Returned is the list of obsolete packs which can then
// be removed.
//
// The map keepBlobs is modified by Repack, it is used to keep track of which
// blobs have been processed.
func Repack(ctx context.Context, repo restic.Repository, dstRepo restic.Repository, packs restic.IDSet, keepBlobs restic.BlobSet, p *progress.Counter) (obsoletePacks restic.IDSet, err error) {
	debug.Log("repacking %d packs while keeping %d blobs", len(packs), len(keepBlobs))

	if repo == dstRepo && dstRepo.Backend().Connections() < 2 {
		return nil, errors.Fatal("repack step requires a backend connection limit of at least two")
	}

	var keepMutex sync.Mutex
	wg, wgCtx := errgroup.WithContext(ctx)

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
					return err
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

	connectionLimit := dstRepo.Backend().Connections() - 1
	if connectionLimit > numRepackWorkers {
		connectionLimit = numRepackWorkers
	}
	for i := 0; i < int(connectionLimit); i++ {
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
