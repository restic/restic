package repository

import (
	"golang.org/x/sync/errgroup"
)

// RunWorkers runs count instances of workerFunc using an errgroup.Group.
// After all workers have terminated, finalFunc is run. If an error occurs in
// one of the workers, it is returned. FinalFunc is always run, regardless of
// any other previous errors.
func RunWorkers(count int, workerFunc func() error, finalFunc func()) error {
	var wg errgroup.Group

	// run workers
	for i := 0; i < count; i++ {
		wg.Go(workerFunc)
	}

	// wait for termination
	err := wg.Wait()

	// make sure finalFunc is run
	finalFunc()

	// return error from workers to the caller
	return err
}
