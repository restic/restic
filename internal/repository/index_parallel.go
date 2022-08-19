package repository

import (
	"context"
	"runtime"
	"sync"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/sync/errgroup"
)

// ForAllIndexes loads all index files in parallel and calls the given callback.
// It is guaranteed that the function is not run concurrently. If the callback
// returns an error, this function is cancelled and also returns that error.
func ForAllIndexes(ctx context.Context, repo restic.Repository,
	fn func(id restic.ID, index *Index, oldFormat bool, err error) error) error {

	debug.Log("Start")

	type FileInfo struct {
		restic.ID
		Size int64
	}

	var m sync.Mutex

	// track spawned goroutines using wg, create a new context which is
	// cancelled as soon as an error occurs.
	wg, ctx := errgroup.WithContext(ctx)

	ch := make(chan FileInfo)
	// send list of index files through ch, which is closed afterwards
	wg.Go(func() error {
		defer close(ch)
		return repo.List(ctx, restic.IndexFile, func(id restic.ID, size int64) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case ch <- FileInfo{id, size}:
			}
			return nil
		})
	})

	// a worker receives an index ID from ch, loads the index, and sends it to indexCh
	worker := func() error {
		var buf []byte
		for fi := range ch {
			debug.Log("worker got file %v", fi.ID.Str())
			var err error
			var idx *Index
			oldFormat := false

			if cap(buf) < int(fi.Size) {
				// overallocate a bit
				buf = make([]byte, fi.Size+128*1024)
			}
			buf, err = repo.LoadUnpacked(ctx, restic.IndexFile, fi.ID, buf[:0])
			if err == nil {
				idx, oldFormat, err = DecodeIndex(buf, fi.ID)
			}

			m.Lock()
			err = fn(fi.ID, idx, oldFormat, err)
			m.Unlock()
			if err != nil {
				return err
			}
		}
		return nil
	}

	// decoding an index can take quite some time such that this can be both CPU- or IO-bound
	// as the whole index is kept in memory anyways, a few workers too much don't matter
	workerCount := int(repo.Connections()) + runtime.GOMAXPROCS(0)
	// run workers on ch
	for i := 0; i < workerCount; i++ {
		wg.Go(worker)
	}

	return wg.Wait()
}
