package restic

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/restic/restic/internal/errors"

	"github.com/restic/restic/internal/debug"
)

// Lock represents a process locking the repository for an operation.
//
// There are two types of locks: exclusive and non-exclusive. There may be many
// different non-exclusive locks, but at most one exclusive lock, which can
// only be acquired while no non-exclusive lock is held.
//
// A lock must be refreshed regularly to not be considered stale, this must be
// triggered by regularly calling Refresh.
type Lock struct {
	lock      sync.Mutex
	Time      time.Time `json:"time"`
	Exclusive bool      `json:"exclusive"`
	Hostname  string    `json:"hostname"`
	Username  string    `json:"username"`
	PID       int       `json:"pid"`
	UID       uint32    `json:"uid,omitempty"`
	GID       uint32    `json:"gid,omitempty"`

	repo   Repository
	lockID *ID
}

// alreadyLockedError is returned when NewLock or NewExclusiveLock are unable to
// acquire the desired lock.
type alreadyLockedError struct {
	otherLock *Lock
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

// invalidLockError is returned when NewLock or NewExclusiveLock fail due
// to an invalid lock.
type invalidLockError struct {
	err error
}

func (e *invalidLockError) Error() string {
	return fmt.Sprintf("invalid lock file: %v", e.err)
}

func (e *invalidLockError) Unwrap() error {
	return e.err
}

// IsInvalidLock returns true iff err indicates that locking failed due to
// an invalid lock.
func IsInvalidLock(err error) bool {
	var e *invalidLockError
	return errors.As(err, &e)
}

// NewLock returns a new, non-exclusive lock for the repository. If an
// exclusive lock is already held by another process, it returns an error
// that satisfies IsAlreadyLocked.
func NewLock(ctx context.Context, repo Repository) (*Lock, error) {
	return newLock(ctx, repo, false)
}

// NewExclusiveLock returns a new, exclusive lock for the repository. If
// another lock (normal and exclusive) is already held by another process,
// it returns an error that satisfies IsAlreadyLocked.
func NewExclusiveLock(ctx context.Context, repo Repository) (*Lock, error) {
	return newLock(ctx, repo, true)
}

var waitBeforeLockCheck = 200 * time.Millisecond

// TestSetLockTimeout can be used to reduce the lock wait timeout for tests.
func TestSetLockTimeout(t testing.TB, d time.Duration) {
	t.Logf("setting lock timeout to %v", d)
	waitBeforeLockCheck = d
}

func newLock(ctx context.Context, repo Repository, excl bool) (*Lock, error) {
	lock := &Lock{
		Time:      time.Now(),
		PID:       os.Getpid(),
		Exclusive: excl,
		repo:      repo,
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
		_ = lock.Unlock()
		return nil, err
	}

	return lock, nil
}

func (l *Lock) fillUserInfo() error {
	usr, err := user.Current()
	if err != nil {
		return nil
	}
	l.Username = usr.Username

	l.UID, l.GID, err = uidGidInt(usr)
	return err
}

// checkForOtherLocks looks for other locks that currently exist in the repository.
//
// If an exclusive lock is to be created, checkForOtherLocks returns an error
// if there are any other locks, regardless if exclusive or not. If a
// non-exclusive lock is to be created, an error is only returned when an
// exclusive lock is found.
func (l *Lock) checkForOtherLocks(ctx context.Context) error {
	var err error
	// retry locking a few times
	for i := 0; i < 3; i++ {
		err = ForAllLocks(ctx, l.repo, l.lockID, func(id ID, lock *Lock, err error) error {
			if err != nil {
				// if we cannot load a lock then it is unclear whether it can be ignored
				// it could either be invalid or just unreadable due to network/permission problems
				debug.Log("ignore lock %v: %v", id, err)
				return err
			}

			if l.Exclusive {
				return &alreadyLockedError{otherLock: lock}
			}

			if !l.Exclusive && lock.Exclusive {
				return &alreadyLockedError{otherLock: lock}
			}

			return nil
		})
		// no lock detected
		if err == nil {
			return nil
		}
		// lock conflicts are permanent
		if _, ok := err.(*alreadyLockedError); ok {
			return err
		}
	}
	if errors.Is(err, ErrInvalidData) {
		return &invalidLockError{err}
	}
	return err
}

// createLock acquires the lock by creating a file in the repository.
func (l *Lock) createLock(ctx context.Context) (ID, error) {
	id, err := SaveJSONUnpacked(ctx, l.repo, LockFile, l)
	if err != nil {
		return ID{}, err
	}

	return id, nil
}

// Unlock removes the lock from the repository.
func (l *Lock) Unlock() error {
	if l == nil || l.lockID == nil {
		return nil
	}

	return l.repo.Backend().Remove(context.TODO(), Handle{Type: LockFile, Name: l.lockID.String()})
}

var StaleLockTimeout = 30 * time.Minute

// Stale returns true if the lock is stale. A lock is stale if the timestamp is
// older than 30 minutes or if it was created on the current machine and the
// process isn't alive any more.
func (l *Lock) Stale() bool {
	l.lock.Lock()
	defer l.lock.Unlock()
	debug.Log("testing if lock %v for process %d is stale", l.lockID, l.PID)
	if time.Since(l.Time) > StaleLockTimeout {
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

// Refresh refreshes the lock by creating a new file in the backend with a new
// timestamp. Afterwards the old lock is removed.
func (l *Lock) Refresh(ctx context.Context) error {
	debug.Log("refreshing lock %v", l.lockID)
	l.lock.Lock()
	l.Time = time.Now()
	l.lock.Unlock()
	id, err := l.createLock(ctx)
	if err != nil {
		return err
	}

	l.lock.Lock()
	defer l.lock.Unlock()

	debug.Log("new lock ID %v", id)
	oldLockID := l.lockID
	l.lockID = &id

	return l.repo.Backend().Remove(context.TODO(), Handle{Type: LockFile, Name: oldLockID.String()})
}

func (l *Lock) String() string {
	l.lock.Lock()
	defer l.lock.Unlock()

	text := fmt.Sprintf("PID %d on %s by %s (UID %d, GID %d)\nlock was created at %s (%s ago)\nstorage ID %v",
		l.PID, l.Hostname, l.Username, l.UID, l.GID,
		l.Time.Format("2006-01-02 15:04:05"), time.Since(l.Time),
		l.lockID.Str())

	return text
}

// listen for incoming SIGHUP and ignore
var ignoreSIGHUP sync.Once

func init() {
	ignoreSIGHUP.Do(func() {
		go func() {
			c := make(chan os.Signal, 1)
			signal.Notify(c, syscall.SIGHUP)
			for s := range c {
				debug.Log("Signal received: %v\n", s)
			}
		}()
	})
}

// LoadLock loads and unserializes a lock from a repository.
func LoadLock(ctx context.Context, repo Repository, id ID) (*Lock, error) {
	lock := &Lock{}
	if err := LoadJSONUnpacked(ctx, repo, LockFile, id, lock); err != nil {
		return nil, err
	}
	lock.lockID = &id

	return lock, nil
}

// RemoveStaleLocks deletes all locks detected as stale from the repository.
func RemoveStaleLocks(ctx context.Context, repo Repository) (uint, error) {
	var processed uint
	err := ForAllLocks(ctx, repo, nil, func(id ID, lock *Lock, err error) error {
		if err != nil {
			// ignore locks that cannot be loaded
			debug.Log("ignore lock %v: %v", id, err)
			return nil
		}

		if lock.Stale() {
			err = repo.Backend().Remove(ctx, Handle{Type: LockFile, Name: id.String()})
			if err == nil {
				processed++
			}
			return err
		}

		return nil
	})
	return processed, err
}

// RemoveAllLocks removes all locks forcefully.
func RemoveAllLocks(ctx context.Context, repo Repository) (uint, error) {
	var processed uint32
	err := ParallelList(ctx, repo.Backend(), LockFile, repo.Connections(), func(ctx context.Context, id ID, size int64) error {
		err := repo.Backend().Remove(ctx, Handle{Type: LockFile, Name: id.String()})
		if err == nil {
			atomic.AddUint32(&processed, 1)
		}
		return err
	})
	return uint(processed), err
}

// ForAllLocks reads all locks in parallel and calls the given callback.
// It is guaranteed that the function is not run concurrently. If the
// callback returns an error, this function is cancelled and also returns that error.
// If a lock ID is passed via excludeID, it will be ignored.
func ForAllLocks(ctx context.Context, repo Repository, excludeID *ID, fn func(ID, *Lock, error) error) error {
	var m sync.Mutex

	// For locks decoding is nearly for free, thus just assume were only limited by IO
	return ParallelList(ctx, repo.Backend(), LockFile, repo.Connections(), func(ctx context.Context, id ID, size int64) error {
		if excludeID != nil && id.Equal(*excludeID) {
			return nil
		}
		if size == 0 {
			// Ignore empty lock files as some backends do not guarantee atomic uploads.
			// These may leave empty files behind if an upload was interrupted between
			// creating the file and writing its data.
			return nil
		}
		lock, err := LoadLock(ctx, repo, id)

		m.Lock()
		defer m.Unlock()
		return fn(id, lock, err)
	})
}
