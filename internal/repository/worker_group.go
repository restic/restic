package repository

import (
	"context"

	"golang.org/x/sync/errgroup"
)

// RunWorkers runs count instances of workerFunc using an errgroup.Group.
// After all workers have terminated, finalFunc is run. If an error occurs in
// one of the workers, it is returned. FinalFunc is always run, regardless of
// any other previous errors.
func RunWorkers(ctx context.Context, count int, workerFunc, finalFunc func() error) error {
	wg, ctx := errgroup.WithContext(ctx)

	// run workers
	for i := 0; i < count; i++ {
		wg.Go(workerFunc)
	}

	// wait for termination
	err := wg.Wait()

	// make sure finalFunc is run
	finalErr := finalFunc()

	// if the workers returned an error, return it to the caller (disregarding
	// any error from finalFunc)
	if err != nil {
		return err
	}

	// if not, return the value finalFunc returned
	return finalErr
}
