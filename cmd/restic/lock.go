package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
)

type lockContext struct {
	lock      *restic.Lock
	cancel    context.CancelFunc
	refreshWG sync.WaitGroup
}

var globalLocks struct {
	locks map[*restic.Lock]*lockContext
	sync.Mutex
	sync.Once
}

func lockRepo(ctx context.Context, repo restic.Repository, retryLock time.Duration, json bool) (*restic.Lock, context.Context, error) {
	return lockRepository(ctx, repo, false, retryLock, json)
}

func lockRepoExclusive(ctx context.Context, repo restic.Repository, retryLock time.Duration, json bool) (*restic.Lock, context.Context, error) {
	return lockRepository(ctx, repo, true, retryLock, json)
}

var (
	retrySleepStart = 5 * time.Second
	retrySleepMax   = 60 * time.Second
)

func minDuration(a, b time.Duration) time.Duration {
	if a <= b {
		return a
	}
	return b
}

// lockRepository wraps the ctx such that it is cancelled when the repository is unlocked
// cancelling the original context also stops the lock refresh
func lockRepository(ctx context.Context, repo restic.Repository, exclusive bool, retryLock time.Duration, json bool) (*restic.Lock, context.Context, error) {
	// make sure that a repository is unlocked properly and after cancel() was
	// called by the cleanup handler in global.go
	globalLocks.Do(func() {
		AddCleanupHandler(unlockAll)
	})

	lockFn := restic.NewLock
	if exclusive {
		lockFn = restic.NewExclusiveLock
	}

	var lock *restic.Lock
	var err error

	retrySleep := minDuration(retrySleepStart, retryLock)
	retryMessagePrinted := false
	retryTimeout := time.After(retryLock)

retryLoop:
	for {
		lock, err = lockFn(ctx, repo)
		if err != nil && restic.IsAlreadyLocked(err) {

			if !retryMessagePrinted {
				if !json {
					Verbosef("repo already locked, waiting up to %s for the lock\n", retryLock)
				}
				retryMessagePrinted = true
			}

			debug.Log("repo already locked, retrying in %v", retrySleep)
			retrySleepCh := time.After(retrySleep)

			select {
			case <-ctx.Done():
				return nil, ctx, ctx.Err()
			case <-retryTimeout:
				debug.Log("repo already locked, timeout expired")
				// Last lock attempt
				lock, err = lockFn(ctx, repo)
				break retryLoop
			case <-retrySleepCh:
				retrySleep = minDuration(retrySleep*2, retrySleepMax)
			}
		} else {
			// anything else, either a successful lock or another error
			break retryLoop
		}
	}
	if restic.IsInvalidLock(err) {
		return nil, ctx, errors.Fatalf("%v\n\nthe `unlock --remove-all` command can be used to remove invalid locks. Make sure that no other restic process is accessing the repository when running the command", err)
	}
	if err != nil {
		return nil, ctx, fmt.Errorf("unable to create lock in backend: %w", err)
	}
	debug.Log("create lock %p (exclusive %v)", lock, exclusive)

	ctx, cancel := context.WithCancel(ctx)
	lockInfo := &lockContext{
		lock:   lock,
		cancel: cancel,
	}
	lockInfo.refreshWG.Add(2)
	refreshChan := make(chan struct{})
	forceRefreshChan := make(chan refreshLockRequest)

	globalLocks.Lock()
	globalLocks.locks[lock] = lockInfo
	go refreshLocks(ctx, repo.Backend(), lockInfo, refreshChan, forceRefreshChan)
	go monitorLockRefresh(ctx, lockInfo, refreshChan, forceRefreshChan)
	globalLocks.Unlock()

	return lock, ctx, err
}

var refreshInterval = 5 * time.Minute

// consider a lock refresh failed a bit before the lock actually becomes stale
// the difference allows to compensate for a small time drift between clients.
var refreshabilityTimeout = restic.StaleLockTimeout - refreshInterval*3/2

type refreshLockRequest struct {
	result chan bool
}

func refreshLocks(ctx context.Context, backend restic.Backend, lockInfo *lockContext, refreshed chan<- struct{}, forceRefresh <-chan refreshLockRequest) {
	debug.Log("start")
	lock := lockInfo.lock
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

		case req := <-forceRefresh:
			debug.Log("trying to refresh stale lock")
			// keep on going if our current lock still exists
			success := tryRefreshStaleLock(ctx, backend, lock, lockInfo.cancel)
			// inform refresh goroutine about forced refresh
			select {
			case <-ctx.Done():
			case req.result <- success:
			}

			if success {
				// update lock refresh time
				lastRefresh = lock.Time
			}

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
				// inform monitor goroutine about successful refresh
				select {
				case <-ctx.Done():
				case refreshed <- struct{}{}:
				}
			}
		}
	}
}

func monitorLockRefresh(ctx context.Context, lockInfo *lockContext, refreshed <-chan struct{}, forceRefresh chan<- refreshLockRequest) {
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
	ticker := time.NewTicker(pollDuration)
	defer func() {
		ticker.Stop()
		lockInfo.cancel()
		lockInfo.refreshWG.Done()
	}()

	var refreshStaleLockResult chan bool

	for {
		select {
		case <-ctx.Done():
			debug.Log("terminate expiry monitoring")
			return
		case <-refreshed:
			if refreshStaleLockResult != nil {
				// ignore delayed refresh notifications while the stale lock is refreshed
				continue
			}
			lastRefresh = time.Now().UnixNano()
		case <-ticker.C:
			if time.Now().UnixNano()-lastRefresh < refreshabilityTimeout.Nanoseconds() || refreshStaleLockResult != nil {
				continue
			}

			debug.Log("trying to refreshStaleLock")
			// keep on going if our current lock still exists
			refreshReq := refreshLockRequest{
				result: make(chan bool),
			}
			refreshStaleLockResult = refreshReq.result

			// inform refresh goroutine about forced refresh
			select {
			case <-ctx.Done():
			case forceRefresh <- refreshReq:
			}
		case success := <-refreshStaleLockResult:
			if success {
				lastRefresh = time.Now().UnixNano()
				refreshStaleLockResult = nil
				continue
			}

			Warnf("Fatal: failed to refresh lock in time\n")
			return
		}
	}
}

func tryRefreshStaleLock(ctx context.Context, backend restic.Backend, lock *restic.Lock, cancel context.CancelFunc) bool {
	freeze := restic.AsBackend[restic.FreezeBackend](backend)
	if freeze != nil {
		debug.Log("freezing backend")
		freeze.Freeze()
		defer freeze.Unfreeze()
	}

	err := lock.RefreshStaleLock(ctx)
	if err != nil {
		Warnf("failed to refresh stale lock: %v\n", err)
		// cancel context while the backend is still frozen to prevent accidental modifications
		cancel()
		return false
	}

	return true
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
