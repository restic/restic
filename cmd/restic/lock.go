package main

import (
	"context"
	"sync"
	"time"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
)

type lockContext struct {
	cancel    context.CancelFunc
	refreshWG sync.WaitGroup
}

var globalLocks struct {
	locks map[*restic.Lock]*lockContext
	sync.Mutex
	sync.Once
}

func lockRepo(ctx context.Context, repo restic.Repository) (*restic.Lock, context.Context, error) {
	return lockRepository(ctx, repo, false)
}

func lockRepoExclusive(ctx context.Context, repo restic.Repository) (*restic.Lock, context.Context, error) {
	return lockRepository(ctx, repo, true)
}

// lockRepository wraps the ctx such that it is cancelled when the repository is unlocked
// cancelling the original context also stops the lock refresh
func lockRepository(ctx context.Context, repo restic.Repository, exclusive bool) (*restic.Lock, context.Context, error) {
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
	if restic.IsInvalidLock(err) {
		return nil, ctx, errors.Fatalf("%v\n\nthe `unlock --remove-all` command can be used to remove invalid locks. Make sure that no other restic process is accessing the repository when running the command", err)
	}
	if err != nil {
		return nil, ctx, errors.Fatalf("unable to create lock in backend: %v", err)
	}
	debug.Log("create lock %p (exclusive %v)", lock, exclusive)

	ctx, cancel := context.WithCancel(ctx)
	lockInfo := &lockContext{
		cancel: cancel,
	}
	lockInfo.refreshWG.Add(2)
	refreshChan := make(chan struct{})

	globalLocks.Lock()
	globalLocks.locks[lock] = lockInfo
	go refreshLocks(ctx, lock, lockInfo, refreshChan)
	go monitorLockRefresh(ctx, lock, lockInfo, refreshChan)
	globalLocks.Unlock()

	return lock, ctx, err
}

var refreshInterval = 5 * time.Minute

// consider a lock refresh failed a bit before the lock actually becomes stale
// the difference allows to compensate for a small time drift between clients.
var refreshabilityTimeout = restic.StaleLockTimeout - refreshInterval*3/2

func refreshLocks(ctx context.Context, lock *restic.Lock, lockInfo *lockContext, refreshed chan<- struct{}) {
	debug.Log("start")
	ticker := time.NewTicker(refreshInterval)
	lastRefresh := lock.Time

	defer func() {
		ticker.Stop()
		// ensure that the context was cancelled before removing the lock
		lockInfo.cancel()

		// remove the lock from the repo
		debug.Log("unlocking repository with lock %v", lock)
		if err := lock.Unlock(); err != nil {
			debug.Log("error while unlocking: %v", err)
			Warnf("error while unlocking: %v", err)
		}

		lockInfo.refreshWG.Done()
	}()

	for {
		select {
		case <-ctx.Done():
			debug.Log("terminate")
			return
		case <-ticker.C:
			if time.Since(lastRefresh) > refreshabilityTimeout {
				// the lock is too old, wait until the expiry monitor cancels the context
				continue
			}

			debug.Log("refreshing locks")
			err := lock.Refresh(context.TODO())
			if err != nil {
				Warnf("unable to refresh lock: %v\n", err)
			} else {
				lastRefresh = lock.Time
				// inform monitor gorountine about successful refresh
				select {
				case <-ctx.Done():
				case refreshed <- struct{}{}:
				}
			}
		}
	}
}

func monitorLockRefresh(ctx context.Context, lock *restic.Lock, lockInfo *lockContext, refreshed <-chan struct{}) {
	// time.Now() might use a monotonic timer which is paused during standby
	// convert to unix time to ensure we compare real time values
	lastRefresh := time.Now().UnixNano()
	pollDuration := 1 * time.Second
	if refreshInterval < pollDuration {
		// require for TestLockFailedRefresh
		pollDuration = refreshInterval / 5
	}
	// timers are paused during standby, which is a problem as the refresh timeout
	// _must_ expire if the host was too long in standby. Thus fall back to periodic checks
	// https://github.com/golang/go/issues/35012
	timer := time.NewTimer(pollDuration)
	defer func() {
		timer.Stop()
		lockInfo.cancel()
		lockInfo.refreshWG.Done()
	}()

	for {
		select {
		case <-ctx.Done():
			debug.Log("terminate expiry monitoring")
			return
		case <-refreshed:
			lastRefresh = time.Now().UnixNano()
		case <-timer.C:
			if time.Now().UnixNano()-lastRefresh < refreshabilityTimeout.Nanoseconds() {
				// restart timer
				timer.Reset(pollDuration)
				continue
			}

			Warnf("Fatal: failed to refresh lock in time\n")
			return
		}
	}
}

func unlockRepo(lock *restic.Lock) {
	if lock == nil {
		return
	}

	globalLocks.Lock()
	lockInfo, exists := globalLocks.locks[lock]
	delete(globalLocks.locks, lock)
	globalLocks.Unlock()

	if !exists {
		debug.Log("unable to find lock %v in the global list of locks, ignoring", lock)
		return
	}
	lockInfo.cancel()
	lockInfo.refreshWG.Wait()
}

func unlockAll(code int) (int, error) {
	globalLocks.Lock()
	locks := globalLocks.locks
	debug.Log("unlocking %d locks", len(globalLocks.locks))
	for _, lockInfo := range globalLocks.locks {
		lockInfo.cancel()
	}
	globalLocks.locks = make(map[*restic.Lock]*lockContext)
	globalLocks.Unlock()

	for _, lockInfo := range locks {
		lockInfo.refreshWG.Wait()
	}

	return code, nil
}

func init() {
	globalLocks.locks = make(map[*restic.Lock]*lockContext)
}
