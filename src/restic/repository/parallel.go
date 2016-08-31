package repository

import (
	"restic"
	"sync"

	"restic/backend"
	"restic/debug"
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

// ParallelWorkFunc gets one file ID to work on. If an error is returned,
// processing stops. If done is closed, the function should return.
type ParallelWorkFunc func(id string, done <-chan struct{}) error

// ParallelIDWorkFunc gets one backend.ID to work on. If an error is returned,
// processing stops. If done is closed, the function should return.
type ParallelIDWorkFunc func(id restic.ID, done <-chan struct{}) error

// FilesInParallel runs n workers of f in parallel, on the IDs that
// repo.List(t) yield. If f returns an error, the process is aborted and the
// first error is returned.
func FilesInParallel(repo backend.Lister, t restic.FileType, n uint, f ParallelWorkFunc) error {
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
				case id, ok := <-ch:
					if !ok {
						return
					}

					err := f(id, done)
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

// ParallelWorkFuncParseID converts a function that takes a backend.ID to a
// function that takes a string. Filenames that do not parse as a backend.ID
// are ignored.
func ParallelWorkFuncParseID(f ParallelIDWorkFunc) ParallelWorkFunc {
	return func(s string, done <-chan struct{}) error {
		id, err := backend.ParseID(s)
		if err != nil {
			debug.Log("repository.ParallelWorkFuncParseID", "invalid ID %q: %v", id, err)
			return err
		}

		return f(id, done)
	}
}
