package main

import (
	"context"
	"sync"
	"time"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
)

var globalLocks struct {
	locks         []*restic.Lock
	cancelRefresh chan struct{}
	refreshWG     sync.WaitGroup
	sync.Mutex
	sync.Once
}

func lockRepo(ctx context.Context, repo *repository.Repository) (*restic.Lock, error) {
	return lockRepository(ctx, repo, false)
}

func lockRepoExclusive(ctx context.Context, repo *repository.Repository) (*restic.Lock, error) {
	return lockRepository(ctx, repo, true)
}

func lockRepository(ctx context.Context, repo *repository.Repository, exclusive bool) (*restic.Lock, error) {
	// make sure that a repository is unlocked properly and after cancel() was
	// called by the cleanup handler in global.go
	globalLocks.Do(func() {
		AddCleanupHandler(unlockAll)
	})

	lockFn := restic.NewLock
	if exclusive {
		lockFn = restic.NewExclusiveLock
	}

	lock, err := lockFn(ctx, repo)
	if err != nil {
		return nil, errors.WithMessage(err, "unable to create lock in backend")
	}
	debug.Log("create lock %p (exclusive %v)", lock, exclusive)

	globalLocks.Lock()
	if globalLocks.cancelRefresh == nil {
		debug.Log("start goroutine for lock refresh")
		globalLocks.cancelRefresh = make(chan struct{})
		globalLocks.refreshWG = sync.WaitGroup{}
		globalLocks.refreshWG.Add(1)
		go refreshLocks(&globalLocks.refreshWG, globalLocks.cancelRefresh)
	}

	globalLocks.locks = append(globalLocks.locks, lock)
	globalLocks.Unlock()

	return lock, err
}

var refreshInterval = 5 * time.Minute

func refreshLocks(wg *sync.WaitGroup, done <-chan struct{}) {
	debug.Log("start")
	defer func() {
		wg.Done()
		globalLocks.Lock()
		globalLocks.cancelRefresh = nil
		globalLocks.Unlock()
	}()

	ticker := time.NewTicker(refreshInterval)

	for {
		select {
		case <-done:
			debug.Log("terminate")
			return
		case <-ticker.C:
			debug.Log("refreshing locks")
			globalLocks.Lock()
			for _, lock := range globalLocks.locks {
				err := lock.Refresh(context.TODO())
				if err != nil {
					Warnf("unable to refresh lock: %v\n", err)
				}
			}
			globalLocks.Unlock()
		}
	}
}

func unlockRepo(lock *restic.Lock) {
	if lock == nil {
		return
	}

	globalLocks.Lock()
	defer globalLocks.Unlock()

	for i := 0; i < len(globalLocks.locks); i++ {
		if lock == globalLocks.locks[i] {
			// remove the lock from the repo
			debug.Log("unlocking repository with lock %v", lock)
			if err := lock.Unlock(); err != nil {
				debug.Log("error while unlocking: %v", err)
				Warnf("error while unlocking: %v", err)
				return
			}

			// remove the lock from the list of locks
			globalLocks.locks = append(globalLocks.locks[:i], globalLocks.locks[i+1:]...)
			return
		}
	}

	debug.Log("unable to find lock %v in the global list of locks, ignoring", lock)
}

func unlockAll(code int) (int, error) {
	globalLocks.Lock()
	defer globalLocks.Unlock()

	debug.Log("unlocking %d locks", len(globalLocks.locks))
	for _, lock := range globalLocks.locks {
		if err := lock.Unlock(); err != nil {
			debug.Log("error while unlocking: %v", err)
			return code, err
		}
		debug.Log("successfully removed lock")
	}
	globalLocks.locks = globalLocks.locks[:0]

	return code, nil
}
