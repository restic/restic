package repository

import (
	"context"
	"sync"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/feature"
	"github.com/restic/restic/internal/repository/index"
	"github.com/restic/restic/internal/repository/pack"
	"github.com/restic/restic/internal/restic"

	"golang.org/x/sync/errgroup"
)

type repackBlobSet interface {
	Has(bh restic.BlobHandle) bool
	Delete(bh restic.BlobHandle)
	Len() int
}

type LogFunc func(msg string, args ...interface{})

// CopyBlobs takes a list of packs together with a list of blobs contained in
// these packs. Each pack is loaded and the blobs listed in keepBlobs is saved
// into a new pack. Returned is the list of obsolete packs which can then
// be removed.
//
// The map keepBlobs is modified by CopyBlobs, it is used to keep track of which
// blobs have been processed.
func CopyBlobs(
	ctx context.Context,
	repo *Repository,
	dstRepo restic.Repository,
	dstUploader restic.BlobSaverWithAsync,
	packs restic.IDSet,
	keepBlobs repackBlobSet,
	p restic.Counter,
	logf LogFunc,
) error {
	return copyBlobs(ctx, repo, dstRepo, dstUploader, packs, keepBlobs, nil, p, logf)
}

// copyBlobs is the implementation of CopyBlobs with optional grouping. When
// treeGroups is set and the uploader supports it, repacked tree blobs are
// clustered into pack files per group id (used by `prune --group-by`).
func copyBlobs(
	ctx context.Context,
	repo *Repository,
	dstRepo restic.Repository,
	dstUploader restic.BlobSaverWithAsync,
	packs restic.IDSet,
	keepBlobs repackBlobSet,
	treeGroups map[restic.BlobHandle]uint32,
	p restic.Counter,
	logf LogFunc,
) error {
	debug.Log("repacking %d packs while keeping %d blobs", len(packs), keepBlobs.Len())

	if logf == nil {
		logf = func(_ string, _ ...interface{}) {}
	}
	p.SetMax(uint64(len(packs)))
	defer p.Done()

	if repo == dstRepo && dstRepo.Connections() < 2 {
		return errors.New("repack step requires a backend connection limit of at least two")
	}

	return repack(ctx, repo, dstRepo, dstUploader, packs, keepBlobs, treeGroups, p, logf)
}

func repack(
	ctx context.Context,
	repo *Repository,
	dstRepo restic.Repository,
	uploader restic.BlobSaverWithAsync,
	packs restic.IDSet,
	keepBlobs repackBlobSet,
	treeGroups map[restic.BlobHandle]uint32,
	p restic.Counter,
	logf LogFunc,
) error {

	// grouped repacking clusters tree blobs of the same snapshot group into the
	// same pack files. It is only active when a group assignment was passed and
	// the uploader supports the grouped save extension.
	groupedUploader, _ := uploader.(restic.GroupedBlobSaver)
	useGroups := treeGroups != nil && groupedUploader != nil

	wg, wgCtx := errgroup.WithContext(ctx)

	if feature.Flag.Enabled(feature.S3Restore) {
		job, err := repo.StartWarmup(ctx, packs)
		if err != nil {
			return err
		}
		if job.HandleCount() != 0 {
			logf("warming up %d packs from cold storage, this may take a while...", job.HandleCount())
			if err := job.Wait(ctx); err != nil {
				return err
			}
		}
	}

	var keepMutex sync.Mutex
	downloadQueue := make(chan index.PackBlobs)
	wg.Go(func() error {
		defer close(downloadQueue)
		for pbs := range repo.listPacksFromIndex(wgCtx, packs) {
			var packBlobs pack.Blobs
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
			case downloadQueue <- index.PackBlobs{PackID: pbs.PackID, Blobs: packBlobs}:
			case <-wgCtx.Done():
				return wgCtx.Err()
			}
		}
		return wgCtx.Err()
	})

	worker := func() error {
		for t := range downloadQueue {
			err := repo.loadBlobsFromPack(wgCtx, t.PackID, t.Blobs, func(blob restic.BlobHandle, buf []byte, err error) error {
				if err != nil {
					// a required blob couldn't be retrieved
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
				if useGroups && blob.Type == restic.TreeBlob {
					// treeGroups maps every repacked tree blob except those whose
					// group was demoted to the shared bucket because the group
					// count exceeded the localization budget; a missing entry
					// falls back to group 0 (the shared bucket).
					_, _, _, err = groupedUploader.SaveBlobGrouped(wgCtx, blob.Type, buf, blob.ID, true, treeGroups[blob])
				} else {
					_, _, _, err = uploader.SaveBlob(wgCtx, blob.Type, buf, blob.ID, true)
				}
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

	return wg.Wait()
}
