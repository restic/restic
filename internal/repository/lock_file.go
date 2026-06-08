package repository

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"sync"
	"testing"
	"time"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
)

// unlockCancelDelay bounds the duration how long lock cleanup operations will wait
// if the passed in context was canceled.
const unlockCancelDelay = 1 * time.Minute

// Lock is the in-repository representation of a repository lock file.
// There are two types of locks: exclusive and non-exclusive. There may be many
// different non-exclusive locks, but at most one exclusive lock, which can
// only be acquired while no non-exclusive lock is held.
//
// A lock must be refreshed regularly to not be considered stale.
type Lock struct {
	Time      time.Time `json:"time"`
	Exclusive bool      `json:"exclusive"`
	Hostname  string    `json:"hostname"`
	Username  string    `json:"username"`
	PID       int       `json:"pid"`
	UID       uint32    `json:"uid,omitempty"`
	GID       uint32    `json:"gid,omitempty"`
}

// lockHandle is a reference to a lock file in the repository.
type lockHandle struct {
	mu sync.Mutex
	Lock
	repo   restic.Unpacked[restic.FileType]
	lockID *restic.ID
}

// alreadyLockedError is returned when newLock is unable to acquire the desired lock.
type alreadyLockedError struct {
	otherLock *lockHandle
}

func (e *alreadyLockedError) Error() string {
	s := ""
	if e.otherLock.Exclusive {
		s = "exclusively "
	}
	return fmt.Sprintf("repository is already locked %sby %v", s, e.otherLock)
}

// IsAlreadyLocked returns true iff err indicates that a repository is
// already locked.
func IsAlreadyLocked(err error) bool {
	var e *alreadyLockedError
	return errors.As(err, &e)
}

// invalidLockError is returned when newLock fails due to an invalid lock.
type invalidLockError struct {
	err error
}

func (e *invalidLockError) Error() string {
	return fmt.Sprintf("invalid lock file: %v", e.err)
}

func (e *invalidLockError) Unwrap() error {
	return e.err
}

// isInvalidLock returns true iff err indicates that locking failed due to
// an invalid lock.
func isInvalidLock(err error) bool {
	var e *invalidLockError
	return errors.As(err, &e)
}

var errRemovedLock = errors.New("lock file was removed in the meantime")

var waitBeforeLockCheck = 200 * time.Millisecond

// delay increases by factor 2 on each retry
var initialWaitBetweenLockRetries = 5 * time.Second

// TestSetLockTimeout can be used to reduce the lock wait timeout for tests.
func TestSetLockTimeout(t testing.TB, d time.Duration) {
	t.Logf("setting lock timeout to %v", d)
	waitBeforeLockCheck = d
	initialWaitBetweenLockRetries = d
}

// newLock returns a new lock for the repository. If an
// exclusive lock is already held by another process, it returns an error
// that satisfies IsAlreadyLocked. If the new lock is exclusive, then other
// non-exclusive locks also result in an IsAlreadyLocked error.
func newLock(ctx context.Context, repo restic.Unpacked[restic.FileType], exclusive bool) (*lockHandle, error) {
	lock := &lockHandle{
		Lock: Lock{
			Time:      time.Now(),
			PID:       os.Getpid(),
			Exclusive: exclusive,
		},
		repo: repo,
	}

	hn, err := os.Hostname()
	if err == nil {
		lock.Hostname = hn
	}

	if err = lock.fillUserInfo(); err != nil {
		return nil, err
	}

	if err = lock.checkForOtherLocks(ctx); err != nil {
		return nil, err
	}

	lockID, err := lock.createLock(ctx)
	if err != nil {
		return nil, err
	}

	lock.lockID = &lockID

	time.Sleep(waitBeforeLockCheck)

	if err = lock.checkForOtherLocks(ctx); err != nil {
		_ = lock.unlock(ctx)
		return nil, err
	}

	return lock, nil
}

func (l *lockHandle) fillUserInfo() error {
	usr, err := user.Current()
	if err != nil {
		return nil
	}
	l.Username = usr.Username

	l.UID, l.GID, err = restic.UidGidInt(usr)
	return err
}

// checkForOtherLocks looks for other locks that currently exist in the repository.
//
// If an exclusive lock is to be created, checkForOtherLocks returns an error
// if there are any other locks, regardless if exclusive or not. If a
// non-exclusive lock is to be created, an error is only returned when an
// exclusive lock is found.
func (l *lockHandle) checkForOtherLocks(ctx context.Context) error {
	var err error
	checkedIDs := restic.NewIDSet()
	if l.lockID != nil {
		checkedIDs.Insert(*l.lockID)
	}
	delay := initialWaitBetweenLockRetries
	// retry locking a few times
	for i := 0; i < 4; i++ {
		if i != 0 {
			// sleep between retries to give backend some time to settle
			if err := cancelableDelay(ctx, delay); err != nil {
				return err
			}
			delay *= 2
		}

		// Store updates in new IDSet to prevent data races
		var m sync.Mutex
		newCheckedIDs := checkedIDs.Clone()
		err = forAllLocks(ctx, l.repo, checkedIDs, func(id restic.ID, lock *lockHandle, err error) error {
			if err != nil {
				// if we cannot load a lock then it is unclear whether it can be ignored
				// it could either be invalid or just unreadable due to network/permission problems
				debug.Log("ignore lock %v: %v", id, err)
				return err
			}

			if l.Exclusive || lock.Exclusive {
				return &alreadyLockedError{otherLock: lock}
			}

			// valid locks will remain valid
			m.Lock()
			newCheckedIDs.Insert(id)
			m.Unlock()
			return nil
		})
		checkedIDs = newCheckedIDs
		// no lock detected
		if err == nil {
			return nil
		}
		// lock conflicts are permanent
		if _, ok := err.(*alreadyLockedError); ok {
			return err
		}
	}
	if errors.Is(err, restic.ErrInvalidData) {
		return &invalidLockError{err}
	}
	return err
}

func cancelableDelay(ctx context.Context, delay time.Duration) error {
	// delay next try a bit
	timer := time.NewTimer(delay)
	select {
	case <-ctx.Done():
		timer.Stop()
		return ctx.Err()
	case <-timer.C:
	}
	return nil
}

// createLock acquires the lock by creating a file in the repository.
func (l *lockHandle) createLock(ctx context.Context) (restic.ID, error) {
	id, err := restic.SaveJSONUnpacked(ctx, l.repo, restic.LockFile, &l.Lock)
	if err != nil {
		return restic.ID{}, err
	}

	return id, nil
}

// unlock removes the lock from the repository.
func (l *lockHandle) unlock(ctx context.Context) error {
	if l == nil || l.lockID == nil {
		return nil
	}

	ctx, cancel := delayedCancelContext(ctx, unlockCancelDelay)
	defer cancel()

	return l.repo.RemoveUnpacked(ctx, restic.LockFile, *l.lockID)
}

var staleLockTimeout = 30 * time.Minute

// stale returns true if the lock is stale. A lock is stale if the timestamp is
// older than 30 minutes or if it was created on the current machine and the
// process isn't alive any more.
func (l *lockHandle) stale() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	debug.Log("testing if lock %v for process %d is stale", l.lockID, l.PID)
	if time.Since(l.Time) > staleLockTimeout {
		debug.Log("lock is stale, timestamp is too old: %v\n", l.Time)
		return true
	}

	hn, err := os.Hostname()
	if err != nil {
		debug.Log("unable to find current hostname: %v", err)
		// since we cannot find the current hostname, assume that the lock is
		// not stale.
		return false
	}

	if hn != l.Hostname {
		// lock was created on a different host, assume the lock is not stale.
		return false
	}

	// check if we can reach the process retaining the lock
	exists := l.processExists()
	if !exists {
		debug.Log("could not reach process, %d, lock is probably stale\n", l.PID)
		return true
	}

	debug.Log("lock not stale\n")
	return false
}

func delayedCancelContext(parentCtx context.Context, delay time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		select {
		case <-parentCtx.Done():
		case <-ctx.Done():
			return
		}

		time.Sleep(delay)
		cancel()
	}()

	return ctx, cancel
}

// refresh refreshes the lock by creating a new file in the backend with a new
// timestamp. Afterwards the old lock is removed.
func (l *lockHandle) refresh(ctx context.Context) error {
	debug.Log("refreshing lock %v", l.lockID)
	id, err := l.createReplacementLock(ctx)
	if err != nil {
		return err
	}

	ctx, cancel := delayedCancelContext(ctx, unlockCancelDelay)
	defer cancel()
	return l.adoptReplacementLock(ctx, id)
}

func (l *lockHandle) createReplacementLock(ctx context.Context) (restic.ID, error) {
	l.mu.Lock()
	l.Time = time.Now()
	l.mu.Unlock()
	return l.createLock(ctx)
}

func (l *lockHandle) adoptReplacementLock(ctx context.Context, id restic.ID) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	debug.Log("new lock ID %v", id)
	oldID := *l.lockID
	l.lockID = &id

	return l.repo.RemoveUnpacked(ctx, restic.LockFile, oldID)
}

// refreshStaleLock is an extended variant of refresh that can also refresh stale lock files.
func (l *lockHandle) refreshStaleLock(ctx context.Context) error {
	debug.Log("refreshing stale lock %v", l.lockID)
	// refreshing a stale lock is possible if it still exists and continues to do
	// so until after creating a new lock. The initial check avoids creating a new
	// lock file if this lock was already removed in the meantime.
	exists, err := l.checkExistence(ctx)
	if err != nil {
		return err
	} else if !exists {
		return errRemovedLock
	}

	id, err := l.createReplacementLock(ctx)
	if err != nil {
		return err
	}

	time.Sleep(waitBeforeLockCheck)

	exists, err = l.checkExistence(ctx)

	ctx, cancel := delayedCancelContext(ctx, unlockCancelDelay)
	defer cancel()

	if err != nil {
		// cleanup replacement lock
		_ = l.repo.RemoveUnpacked(ctx, restic.LockFile, id)
		return err
	}

	if !exists {
		// cleanup replacement lock
		_ = l.repo.RemoveUnpacked(ctx, restic.LockFile, id)
		return errRemovedLock
	}

	return l.adoptReplacementLock(ctx, id)
}

func (l *lockHandle) checkExistence(ctx context.Context) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	exists := false

	err := l.repo.List(ctx, restic.LockFile, func(id restic.ID, _ int64) error {
		if id.Equal(*l.lockID) {
			exists = true
		}
		return nil
	})

	return exists, err
}

func (l *lockHandle) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()

	text := fmt.Sprintf("PID %d on %s by %s (UID %d, GID %d)\nlock was created at %s (%s ago)\nstorage ID %v",
		l.PID, l.Hostname, l.Username, l.UID, l.GID,
		l.Time.Format("2006-01-02 15:04:05"), time.Since(l.Time),
		l.lockID.Str())

	return text
}

// LoadLock loads and unserializes a lock from a repository.
func LoadLock(ctx context.Context, repo restic.LoaderUnpacked, id restic.ID) (Lock, error) {
	var lock Lock
	err := restic.LoadJSONUnpacked(ctx, repo, restic.LockFile, id, &lock)
	return lock, err
}

// forAllLocks reads all locks in parallel and calls the given callback.
// It is guaranteed that the function is not run concurrently. If the
// callback returns an error, this function is cancelled and also returns that error.
// If a lock ID is passed via excludeID, it will be ignored.
func forAllLocks(ctx context.Context, repo restic.ListerLoaderUnpacked, excludeIDs restic.IDSet, fn func(restic.ID, *lockHandle, error) error) error {
	var m sync.Mutex

	// For locks decoding is nearly for free, thus just assume were only limited by IO
	return restic.ParallelList(ctx, repo, restic.LockFile, repo.Connections(), func(ctx context.Context, id restic.ID, size int64) error {
		if excludeIDs.Has(id) {
			return nil
		}
		if size == 0 {
			// Ignore empty lock files as some backends do not guarantee atomic uploads.
			// These may leave empty files behind if an upload was interrupted between
			// creating the file and writing its data.
			return nil
		}
		lock, err := LoadLock(ctx, repo, id)
		var handle *lockHandle
		if err == nil {
			handle = &lockHandle{Lock: lock, lockID: &id}
		}

		m.Lock()
		defer m.Unlock()
		return fn(id, handle, err)
	})
}
