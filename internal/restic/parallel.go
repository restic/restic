package restic

import (
	"context"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/ui/progress"
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
		return r.List(ctx, t, func(id ID, size int64) error {
			select {
			case <-ctx.Done():
				return nil
			case ch <- FileIDInfo{id, size}:
			}
			return nil
		})
	})

	// a worker receives an index ID from ch, loads the index, and sends it to indexCh
	worker := func() error {
		for fi := range ch {
			debug.Log("worker got file %v/%v", t, fi.ID.Str())
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

// ParallelRemove deletes the given fileList of fileType in parallel
// if callback returns an error, then it will abort.
func ParallelRemove(ctx context.Context, repo RemoverUnpacked, fileList IDSet, fileType FileType, report func(id ID, err error) error, bar *progress.Counter) error {
	fileChan := make(chan ID)
	wg, ctx := errgroup.WithContext(ctx)
	wg.Go(func() error {
		defer close(fileChan)
		for id := range fileList {
			select {
			case fileChan <- id:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})

	bar.SetMax(uint64(len(fileList)))

	// deleting files is IO-bound
	workerCount := repo.Connections()
	for i := 0; i < int(workerCount); i++ {
		wg.Go(func() error {
			for id := range fileChan {
				err := repo.RemoveUnpacked(ctx, fileType, id)
				if report != nil {
					err = report(id, err)
				}
				if err != nil {
					return err
				}
				bar.Add(1)
			}
			return nil
		})
	}
	return wg.Wait()
}
