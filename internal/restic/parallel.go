package restic

import (
	"context"

	"github.com/restic/restic/internal/debug"
	"golang.org/x/sync/errgroup"
)

func ParallelList(ctx context.Context, r Lister, t FileType, parallelism uint, fn func(context.Context, ID, int64) error) error {

	type FileIDInfo struct {
		ID
		Size int64
	}

	// track spawned goroutines using wg, create a new context which is
	// cancelled as soon as an error occurs.
	wg, ctx := errgroup.WithContext(ctx)

	ch := make(chan FileIDInfo)
	// send list of index files through ch, which is closed afterwards
	wg.Go(func() error {
		defer close(ch)
		return r.List(ctx, t, func(fi FileInfo) error {
			id, err := ParseID(fi.Name)
			if err != nil {
				debug.Log("unable to parse %v as an ID", fi.Name)
				return nil
			}

			select {
			case <-ctx.Done():
				return nil
			case ch <- FileIDInfo{id, fi.Size}:
			}
			return nil
		})
	})

	// a worker receives an index ID from ch, loads the index, and sends it to indexCh
	worker := func() error {
		for fi := range ch {
			debug.Log("worker got file %v", fi.ID.Str())
			err := fn(ctx, fi.ID, fi.Size)
			if err != nil {
				return err
			}
		}
		return nil
	}

	// run workers on ch
	for i := uint(0); i < parallelism; i++ {
		wg.Go(worker)
	}

	return wg.Wait()
}
