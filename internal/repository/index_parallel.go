package repository

import (
	"context"
	"sync"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/sync/errgroup"
)

const loadIndexParallelism = 5

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
				return nil
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

			buf, err = repo.LoadAndDecrypt(ctx, buf[:0], restic.IndexFile, fi.ID)
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

	// run workers on ch
	wg.Go(func() error {
		return RunWorkers(loadIndexParallelism, worker)
	})

	return wg.Wait()
}
