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

// ParallelRemove deletes the given fileList of fileType in parallel.
// If report returns an error, it aborts.
func ParallelRemove[FT FileTypes](ctx context.Context, repo RemoverUnpacked[FT], fileList IDSet, fileType FT, report func(id ID, err error) error, bar *progress.Counter) error {
	wg, ctx := errgroup.WithContext(ctx)
	wg.SetLimit(int(repo.Connections())) // deleting files is IO-bound

	bar.SetMax(uint64(len(fileList)))

loop:
	for id := range fileList {
		select {
		case <-ctx.Done():
			break loop
		default:
		}

		wg.Go(func() error {
			err := repo.RemoveUnpacked(ctx, fileType, id)
			if err == nil {
				bar.Add(1)
			}
			if report != nil {
				err = report(id, err)
			}
			return err
		})
	}
	return wg.Wait()
}
