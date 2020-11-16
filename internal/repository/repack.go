package repository

import (
	"context"
	"os"
	"sync"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/pack"
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
func Repack(ctx context.Context, repo restic.Repository, packs restic.IDSet, keepBlobs restic.BlobSet, p *progress.Counter) (obsoletePacks restic.IDSet, err error) {
	debug.Log("repacking %d packs while keeping %d blobs", len(packs), len(keepBlobs))

	wg, wgCtx := errgroup.WithContext(ctx)

	downloadQueue := make(chan restic.ID)
	wg.Go(func() error {
		defer close(downloadQueue)
		for packID := range packs {
			select {
			case downloadQueue <- packID:
			case <-wgCtx.Done():
				return wgCtx.Err()
			}
		}
		return nil
	})

	type repackJob struct {
		tempfile   *os.File
		hash       restic.ID
		packLength int64
	}
	processQueue := make(chan repackJob)
	// used to close processQueue once all downloaders have finished
	var downloadWG sync.WaitGroup

	downloader := func() error {
		defer downloadWG.Done()
		for packID := range downloadQueue {
			// load the complete pack into a temp file
			h := restic.Handle{Type: restic.PackFile, Name: packID.String()}

			tempfile, hash, packLength, err := DownloadAndHash(wgCtx, repo.Backend(), h)
			if err != nil {
				return errors.Wrap(err, "Repack")
			}

			debug.Log("pack %v loaded (%d bytes), hash %v", packID, packLength, hash)

			if !packID.Equal(hash) {
				return errors.Errorf("hash does not match id: want %v, got %v", packID, hash)
			}

			select {
			case processQueue <- repackJob{tempfile, hash, packLength}:
			case <-wgCtx.Done():
				return wgCtx.Err()
			}
		}
		return nil
	}

	downloadWG.Add(numRepackWorkers)
	for i := 0; i < numRepackWorkers; i++ {
		wg.Go(downloader)
	}
	wg.Go(func() error {
		downloadWG.Wait()
		close(processQueue)
		return nil
	})

	var keepMutex sync.Mutex
	worker := func() error {
		for job := range processQueue {
			tempfile, packID, packLength := job.tempfile, job.hash, job.packLength

			blobs, _, err := pack.List(repo.Key(), tempfile, packLength)
			if err != nil {
				return err
			}

			debug.Log("processing pack %v, blobs: %v", packID, len(blobs))
			var buf []byte
			for _, entry := range blobs {
				h := restic.BlobHandle{ID: entry.ID, Type: entry.Type}

				keepMutex.Lock()
				shouldKeep := keepBlobs.Has(h)
				keepMutex.Unlock()

				if !shouldKeep {
					continue
				}

				debug.Log("  process blob %v", h)

				if uint(cap(buf)) < entry.Length {
					buf = make([]byte, entry.Length)
				}
				buf = buf[:entry.Length]

				n, err := tempfile.ReadAt(buf, int64(entry.Offset))
				if err != nil {
					return errors.Wrap(err, "ReadAt")
				}

				if n != len(buf) {
					return errors.Errorf("read blob %v from %v: not enough bytes read, want %v, got %v",
						h, tempfile.Name(), len(buf), n)
				}

				nonce, ciphertext := buf[:repo.Key().NonceSize()], buf[repo.Key().NonceSize():]
				plaintext, err := repo.Key().Open(ciphertext[:0], nonce, ciphertext, nil)
				if err != nil {
					return err
				}

				id := restic.Hash(plaintext)
				if !id.Equal(entry.ID) {
					debug.Log("read blob %v/%v from %v: wrong data returned, hash is %v",
						h.Type, h.ID, tempfile.Name(), id)
					return errors.Errorf("read blob %v from %v: wrong data returned, hash is %v",
						h, tempfile.Name(), id)
				}

				keepMutex.Lock()
				// recheck whether some other worker was faster
				shouldKeep = keepBlobs.Has(h)
				if shouldKeep {
					keepBlobs.Delete(h)
				}
				keepMutex.Unlock()

				if !shouldKeep {
					continue
				}

				// We do want to save already saved blobs!
				_, _, err = repo.SaveBlob(wgCtx, entry.Type, plaintext, entry.ID, true)
				if err != nil {
					return err
				}

				debug.Log("  saved blob %v", entry.ID)
			}

			if err = tempfile.Close(); err != nil {
				return errors.Wrap(err, "Close")
			}

			if err = fs.RemoveIfExists(tempfile.Name()); err != nil {
				return errors.Wrap(err, "Remove")
			}
			p.Add(1)
		}
		return nil
	}

	for i := 0; i < numRepackWorkers; i++ {
		wg.Go(worker)
	}

	if err := wg.Wait(); err != nil {
		return nil, err
	}

	if err := repo.Flush(ctx); err != nil {
		return nil, err
	}

	return packs, nil
}
