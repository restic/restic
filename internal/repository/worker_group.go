package repository

import (
	"golang.org/x/sync/errgroup"
)

// RunWorkers runs count instances of workerFunc using an errgroup.Group.
// If an error occurs in one of the workers, it is returned.
func RunWorkers(count int, workerFunc func() error) error {
	var wg errgroup.Group

	// run workers
	for i := 0; i < count; i++ {
		wg.Go(workerFunc)
	}

	return wg.Wait()
}
