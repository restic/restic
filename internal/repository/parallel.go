package repository

import (
	"context"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/sync/errgroup"
)

// ParallelWorkFunc gets one file ID to work on. If an error is returned,
// processing stops. When the contect is cancelled the function should return.
type ParallelWorkFunc func(ctx context.Context, id string) error

// ParallelIDWorkFunc gets one restic.ID to work on. If an error is returned,
// processing stops. When the context is cancelled the function should return.
type ParallelIDWorkFunc func(ctx context.Context, id restic.ID) error

// FilesInParallel runs n workers of f in parallel, on the IDs that
// repo.List(t) yields. If f returns an error, the process is aborted and the
// first error is returned.
func FilesInParallel(ctx context.Context, repo restic.Lister, t restic.FileType, n int, f ParallelWorkFunc) error {
	g, ctx := errgroup.WithContext(ctx)

	ch := make(chan string, n)
	g.Go(func() error {
		defer close(ch)
		return repo.List(ctx, t, func(fi restic.FileInfo) error {
			select {
			case <-ctx.Done():
			case ch <- fi.Name:
			}
			return nil
		})
	})

	for i := 0; i < n; i++ {
		g.Go(func() error {
			for name := range ch {
				err := f(ctx, name)
				if err != nil {
					return err
				}
			}
			return nil
		})
	}

	return g.Wait()
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
