package main

import (
	"fmt"
	"os"
	"sync"
	"time"

	"restic"
	"restic/debug"
	"restic/repository"
)

var globalLocks struct {
	locks         []*restic.Lock
	cancelRefresh chan struct{}
	refreshWG     sync.WaitGroup
	sync.Mutex
}

func lockRepo(repo *repository.Repository) (*restic.Lock, error) {
	return lockRepository(repo, false)
}

func lockRepoExclusive(repo *repository.Repository) (*restic.Lock, error) {
	return lockRepository(repo, true)
}

func lockRepository(repo *repository.Repository, exclusive bool) (*restic.Lock, error) {
	lockFn := restic.NewLock
	if exclusive {
		lockFn = restic.NewExclusiveLock
	}

	lock, err := lockFn(repo)
	if err != nil {
		return nil, err
	}

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
				err := lock.Refresh()
				if err != nil {
					fmt.Fprintf(os.Stderr, "unable to refresh lock: %v\n", err)
				}
			}
			globalLocks.Unlock()
		}
	}
}

func unlockRepo(lock *restic.Lock) error {
	globalLocks.Lock()
	defer globalLocks.Unlock()

	debug.Log("unlocking repository")
	if err := lock.Unlock(); err != nil {
		debug.Log("error while unlocking: %v", err)
		return err
	}

	for i := 0; i < len(globalLocks.locks); i++ {
		if lock == globalLocks.locks[i] {
			globalLocks.locks = append(globalLocks.locks[:i], globalLocks.locks[i+1:]...)
			return nil
		}
	}

	return nil
}

func unlockAll() error {
	globalLocks.Lock()
	defer globalLocks.Unlock()

	debug.Log("unlocking %d locks", len(globalLocks.locks))
	for _, lock := range globalLocks.locks {
		if err := lock.Unlock(); err != nil {
			debug.Log("error while unlocking: %v", err)
			return err
		}
		debug.Log("successfully removed lock")
	}

	return nil
}

func init() {
	AddCleanupHandler(unlockAll)
}
