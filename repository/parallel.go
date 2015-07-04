package repository

import (
	"sync"

	"github.com/restic/restic/backend"
)

func closeIfOpen(ch chan struct{}) {
	// only close ch when it is not already closed, in which the case statement runs.
	select {
	case <-ch:
		return
	default:
		close(ch)
	}
}

// FilesInParallel runs n workers of f in parallel, on the IDs that
// repo.List(t) yield. If f returns an error, the process is aborted and the
// first error is returned.
func FilesInParallel(repo backend.Lister, t backend.Type, n uint, f func(backend.ID) error) error {
	done := make(chan struct{})
	defer closeIfOpen(done)

	wg := &sync.WaitGroup{}

	ch := repo.List(t, done)

	errors := make(chan error, n)

	for i := 0; uint(i) < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for {
				select {
				case item, ok := <-ch:
					if !ok {
						return
					}

					id, err := backend.ParseID(item)

					if err == nil {
						err = f(id)
					}

					if err != nil {
						closeIfOpen(done)
						errors <- err
						return
					}
				case <-done:
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
