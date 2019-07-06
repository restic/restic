package repository

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/pack"
	"github.com/restic/restic/internal/restic"

	tomb "gopkg.in/tomb.v2"
)

// A simple buffer pool. We can't use the one in internal/archiver due to an
// import cycle.
type bufferPool struct {
	ch chan []byte
}

// Create a new buffer pool, capable of holding up to `max` buffers.
func newBufferPool(max int) *bufferPool {
	return &bufferPool{
		ch: make(chan []byte, max),
	}
}

// Get a buffer of at least the specified size. If the first buffer available in
// the pool is large enough, it will be used; otherwise a new one will be
// created. The returned buffer will have the requested length, but the capacity
// may be larger.
func (pool *bufferPool) Get(size uint) []byte {
	select {
	case buf := <-pool.ch:
		if len(buf) >= int(size) {
			return buf[:size]
		}
	default:
	}

	return make([]byte, size)
}

// Add a buffer to the pool.
func (pool *bufferPool) Put(buf []byte) {
	select {
	case pool.ch <- buf[:cap(buf)]:
	default:
	}
}

// Close the pool, and free all held buffers.
func (pool *bufferPool) Close() {
	close(pool.ch)
	for range pool.ch {
	}
}

// Repack takes a list of packs together with a list of blobs contained in
// these packs. Each pack is loaded and the blobs listed in keepBlobs is saved
// into a new pack. Returned is the list of obsolete packs which can then
// be removed.
func Repack(ctx context.Context, repo restic.Repository, packs restic.IDSet, keepBlobs restic.BlobSet, p *restic.Progress) (obsoletePacks restic.IDSet, err error) {
	debug.Log("repacking %d packs while keeping %d blobs", len(packs), len(keepBlobs))

	t, wctx := tomb.WithContext(ctx)

	p.Start()
	defer p.Done()

	type fetchJob struct {
		packID restic.ID
	}
	fetchCh := make(chan fetchJob)

	type readJob struct {
		packID     restic.ID
		packLength int64
		tempfile   *os.File
	}
	readCh := make(chan readJob)

	type saveJob struct {
		entry     restic.Blob
		plaintext []byte
	}
	saveCh := make(chan saveJob)

	// Feed pack IDs to the rest of the pipeline
	inputWorker := func() error {
		defer close(fetchCh)
		for packID := range packs {
			select {
			case fetchCh <- fetchJob{packID: packID}:
			case <-t.Dying():
				return tomb.ErrDying
			}
		}
		return nil
	}

	// Fetch packs from the repository
	var fetchWG sync.WaitGroup
	fetchWorker := func() error {
		defer fetchWG.Done()
		for job := range fetchCh {
			// load the complete pack into a temp file
			h := restic.Handle{Type: restic.DataFile, Name: job.packID.String()}

			tempfile, hash, packLength, err := DownloadAndHash(wctx, repo.Backend(), h)
			if err != nil {
				return errors.Wrap(err, "Repack")
			}

			debug.Log("pack %v fetched (%d bytes), hash %v", job.packID, packLength, hash)

			if !job.packID.Equal(hash) {
				return errors.Errorf("hash does not match id: want %v, got %v", job.packID, hash)
			}

			_, err = tempfile.Seek(0, 0)
			if err != nil {
				return err
			}

			select {
			case readCh <- readJob{packID: job.packID, packLength: packLength, tempfile: tempfile}:
			case <-t.Dying():
				return tomb.ErrDying
			}
		}
		return nil
	}

	pool := newBufferPool(1000)
	defer pool.Close()

	// Read the blobs from the downloaded pack files
	var readWG sync.WaitGroup
	readWorker := func() error {
		defer readWG.Done()
		for job := range readCh {
			blobs, err := pack.List(repo.Key(), job.tempfile, job.packLength)
			if err != nil {
				return err
			}

			debug.Log("processing pack %v, blobs: %v", job.packID, len(blobs))
			for _, entry := range blobs {
				h := restic.BlobHandle{ID: entry.ID, Type: entry.Type}
				if !keepBlobs.Has(h) {
					continue
				}

				debug.Log("  read blob %v", h)

				buf := pool.Get(entry.Length)
				n, err := job.tempfile.ReadAt(buf, int64(entry.Offset))
				if err != nil {
					return errors.Wrap(err, "ReadAt")
				}

				if n != len(buf) {
					return errors.Errorf("read blob %v from %v: not enough bytes read, want %v, got %v",
						h, job.tempfile.Name(), len(buf), n)
				}

				nonce, ciphertext := buf[:repo.Key().NonceSize()], buf[repo.Key().NonceSize():]
				plaintext, err := repo.Key().Open(ciphertext[:0], nonce, ciphertext, nil)
				if err != nil {
					return err
				}

				id := restic.Hash(plaintext)
				if !id.Equal(entry.ID) {
					debug.Log("read blob %v/%v from %v: wrong data returned, hash is %v",
						h.Type, h.ID, job.tempfile.Name(), id)
					fmt.Fprintf(os.Stderr, "read blob %v from %v: wrong data returned, hash is %v",
						h, job.tempfile.Name(), id)
				}

				select {
				case saveCh <- saveJob{entry: entry, plaintext: plaintext}:
				case <-t.Dying():
					return tomb.ErrDying
				}
			}

			if err = job.tempfile.Close(); err != nil {
				return errors.Wrap(err, "Close")
			}

			if err = fs.RemoveIfExists(job.tempfile.Name()); err != nil {
				return errors.Wrap(err, "Remove")
			}

			p.Report(restic.Stat{Blobs: 1})
		}
		return nil
	}

	// Save the kept blobs back to the repository
	var seenBlobs sync.Map
	saveWorker := func() error {
		for job := range saveCh {
			h := restic.BlobHandle{Type: job.entry.Type, ID: job.entry.ID}
			if _, found := seenBlobs.LoadOrStore(h, struct{}{}); found {
				continue
			}

			debug.Log("write blob %v/%v", job.entry.Type, job.entry.ID)
			id, err := repo.SaveBlob(wctx, job.entry.Type, job.plaintext, restic.ID{})
			if err != nil {
				return err
			}
			if !id.Equal(job.entry.ID) {
				debug.Log("wrote blob %v/%v: wrong data written, hash is %v",
					job.entry.Type, job.entry.ID, id)
				fmt.Fprintf(os.Stderr, "wrote blob %v: wrong data written, hash is %v",
					h, id)
			}
			debug.Log("  saved blob %v", job.entry.ID)

			pool.Put(job.plaintext)

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

		fetchWorkers := repo.Backend().Connections()
		fetchWG.Add(int(fetchWorkers))
		for i := uint(0); i < fetchWorkers; i++ {
			t.Go(fetchWorker)
		}
		t.Go(func() error {
			fetchWG.Wait()
			close(readCh)
			return nil
		})

		readWorkers := runtime.GOMAXPROCS(0)
		readWG.Add(readWorkers)
		for i := 0; i < readWorkers; i++ {
			t.Go(readWorker)
		}
		t.Go(func() error {
			readWG.Wait()
			close(saveCh)
			return nil
		})

		saveWorkers := runtime.GOMAXPROCS(0)
		for i := 0; i < saveWorkers; i++ {
			t.Go(saveWorker)
		}
		return nil
	})

	if err := t.Wait(); err != nil {
		return nil, err
	}

	if err := repo.Flush(ctx); err != nil {
		return nil, err
	}

	debug.Log("finished repacking")

	return packs, nil
}
