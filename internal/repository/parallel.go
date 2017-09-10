package repository

import (
	"context"
	"sync"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
)

// ParallelWorkFunc gets one file ID to work on. If an error is returned,
// processing stops. When the contect is cancelled the function should return.
type ParallelWorkFunc func(ctx context.Context, id string) error

// ParallelIDWorkFunc gets one restic.ID to work on. If an error is returned,
// processing stops. When the context is cancelled the function should return.
type ParallelIDWorkFunc func(ctx context.Context, id restic.ID) error

// FilesInParallel runs n workers of f in parallel, on the IDs that
// repo.List(t) yield. If f returns an error, the process is aborted and the
// first error is returned.
func FilesInParallel(ctx context.Context, repo restic.Lister, t restic.FileType, n uint, f ParallelWorkFunc) error {
	wg := &sync.WaitGroup{}
	ch := repo.List(ctx, t)
	errors := make(chan error, n)

	for i := 0; uint(i) < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for {
				select {
				case id, ok := <-ch:
					if !ok {
						return
					}

					err := f(ctx, id)
					if err != nil {
						errors <- err
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	wg.Wait()

	select {
	case err := <-errors:
		return err
	default:
		break
	}

	return nil
}

// ParallelWorkFuncParseID converts a function that takes a restic.ID to a
// function that takes a string. Filenames that do not parse as a restic.ID
// are ignored.
func ParallelWorkFuncParseID(f ParallelIDWorkFunc) ParallelWorkFunc {
	return func(ctx context.Context, s string) error {
		id, err := restic.ParseID(s)
		if err != nil {
			debug.Log("invalid ID %q: %v", id, err)
			return nil
		}

		return f(ctx, id)
	}
}
