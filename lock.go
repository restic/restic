package restic

import (
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/debug"
	"github.com/restic/restic/repository"
	"runtime"
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
	Time      time.Time `json:"time"`
	Exclusive bool      `json:"exclusive"`
	Hostname  string    `json:"hostname"`
	Username  string    `json:"username"`
	PID       int       `json:"pid"`
	UID       uint32    `json:"uid,omitempty"`
	GID       uint32    `json:"gid,omitempty"`

	repo   *repository.Repository
	lockID *backend.ID
}

// ErrAlreadyLocked is returned when NewLock or NewExclusiveLock are unable to
// acquire the desired lock.
type ErrAlreadyLocked struct {
	otherLock *Lock
}

func (e ErrAlreadyLocked) Error() string {
	return fmt.Sprintf("repository is already locked by %v", e.otherLock)
}

// IsAlreadyLocked returns true iff err is an instance of ErrAlreadyLocked.
func IsAlreadyLocked(err error) bool {
	if _, ok := err.(ErrAlreadyLocked); ok {
		return true
	}

	return false
}

// NewLock returns a new, non-exclusive lock for the repository. If an
// exclusive lock is already held by another process, ErrAlreadyLocked is
// returned.
func NewLock(repo *repository.Repository) (*Lock, error) {
	return newLock(repo, false)
}

// NewExclusiveLock returns a new, exclusive lock for the repository. If
// another lock (normal and exclusive) is already held by another process,
// ErrAlreadyLocked is returned.
func NewExclusiveLock(repo *repository.Repository) (*Lock, error) {
	return newLock(repo, true)
}

const waitBeforeLockCheck = 200 * time.Millisecond

func newLock(repo *repository.Repository, excl bool) (*Lock, error) {
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

	if err = lock.checkForOtherLocks(); err != nil {
		return nil, err
	}

	lockID, err := lock.createLock()
	if err != nil {
		return nil, err
	}

	lock.lockID = &lockID

	time.Sleep(waitBeforeLockCheck)

	if err = lock.checkForOtherLocks(); err != nil {
		lock.Unlock()
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

	// We ignore the error. On Windows Uid is not a number
	uid, _ := strconv.ParseInt(usr.Uid, 10, 32)
	l.UID = uint32(uid)

	// We ignore the error. On Windows Gid is not a number
	gid, _ := strconv.ParseInt(usr.Gid, 10, 32)
	l.GID = uint32(gid)

	return nil
}

// checkForOtherLocks looks for other locks that currently exist in the repository.
//
// If an exclusive lock is to be created, checkForOtherLocks returns an error
// if there are any other locks, regardless if exclusive or not. If a
// non-exclusive lock is to be created, an error is only returned when an
// exclusive lock is found.
func (l *Lock) checkForOtherLocks() error {
	return eachLock(l.repo, func(id backend.ID, lock *Lock, err error) error {
		if l.lockID != nil && id.Equal(*l.lockID) {
			return nil
		}

		// ignore locks that cannot be loaded
		if err != nil {
			return nil
		}

		if l.Exclusive {
			return ErrAlreadyLocked{otherLock: lock}
		}

		if !l.Exclusive && lock.Exclusive {
			return ErrAlreadyLocked{otherLock: lock}
		}

		return nil
	})
}

func eachLock(repo *repository.Repository, f func(backend.ID, *Lock, error) error) error {
	done := make(chan struct{})
	defer close(done)

	for id := range repo.List(backend.Lock, done) {
		lock, err := LoadLock(repo, id)
		err = f(id, lock, err)
		if err != nil {
			return err
		}
	}

	return nil
}

// createLock acquires the lock by creating a file in the repository.
func (l *Lock) createLock() (backend.ID, error) {
	id, err := l.repo.SaveJSONUnpacked(backend.Lock, l)
	if err != nil {
		return backend.ID{}, err
	}

	return id, nil
}

// Unlock removes the lock from the repository.
func (l *Lock) Unlock() error {
	if l == nil || l.lockID == nil {
		return nil
	}

	return l.repo.Backend().Remove(backend.Lock, l.lockID.String())
}

var staleTimeout = 30 * time.Minute

// Stale returns true if the lock is stale. A lock is stale if the timestamp is
// older than 30 minutes or if it was created on the current machine and the
// process isn't alive any more.
func (l *Lock) Stale() bool {
	debug.Log("Lock.Stale", "testing if lock %v for process %d is stale", l.lockID.Str(), l.PID)
	if time.Now().Sub(l.Time) > staleTimeout {
		debug.Log("Lock.Stale", "lock is stale, timestamp is too old: %v\n", l.Time)
		return true
	}

	proc, err := os.FindProcess(l.PID)
	if err != nil {
		debug.Log("Lock.Stale", "error searching for process %d: %v\n", l.PID, err)
		return true
	}
	defer proc.Release()

	// Windows does not have SIGHUP
	if runtime.GOOS != "windows" {
		debug.Log("Lock.Stale", "sending SIGHUP to process %d\n", l.PID)
		err = proc.Signal(syscall.SIGHUP)
		if err != nil {
			debug.Log("Lock.Stale", "signal error: %v, lock is probably stale\n", err)
			return true
		}
	}

	debug.Log("Lock.Stale", "lock not stale\n")
	return false
}

// Refresh refreshes the lock by creating a new file in the backend with a new
// timestamp. Afterwards the old lock is removed.
func (l *Lock) Refresh() error {
	debug.Log("Lock.Refresh", "refreshing lock %v", l.lockID.Str())
	id, err := l.createLock()
	if err != nil {
		return err
	}

	err = l.repo.Backend().Remove(backend.Lock, l.lockID.String())
	if err != nil {
		return err
	}

	debug.Log("Lock.Refresh", "new lock ID %v", id.Str())
	l.lockID = &id

	return nil
}

func (l Lock) String() string {
	text := fmt.Sprintf("PID %d on %s by %s (UID %d, GID %d)\nlock was created at %s (%s ago)\nstorage ID %v",
		l.PID, l.Hostname, l.Username, l.UID, l.GID,
		l.Time.Format("2006-01-02 15:04:05"), time.Since(l.Time),
		l.lockID.Str())

	if l.Stale() {
		text += " (stale)"
	}

	return text
}

// listen for incoming SIGHUP and ignore
var ignoreSIGHUP sync.Once

func init() {
	ignoreSIGHUP.Do(func() {
		go func() {
			c := make(chan os.Signal)
			signal.Notify(c, syscall.SIGHUP)
			for s := range c {
				debug.Log("lock.ignoreSIGHUP", "Signal received: %v\n", s)
			}
		}()
	})
}

// LoadLock loads and unserializes a lock from a repository.
func LoadLock(repo *repository.Repository, id backend.ID) (*Lock, error) {
	lock := &Lock{}
	if err := repo.LoadJSONUnpacked(backend.Lock, id, lock); err != nil {
		return nil, err
	}
	lock.lockID = &id

	return lock, nil
}

// RemoveStaleLocks deletes all locks detected as stale from the repository.
func RemoveStaleLocks(repo *repository.Repository) error {
	return eachLock(repo, func(id backend.ID, lock *Lock, err error) error {
		// ignore locks that cannot be loaded
		if err != nil {
			return nil
		}

		if lock.Stale() {
			return repo.Backend().Remove(backend.Lock, id.String())
		}

		return nil
	})
}

// RemoveAllLocks removes all locks forcefully.
func RemoveAllLocks(repo *repository.Repository) error {
	return eachLock(repo, func(id backend.ID, lock *Lock, err error) error {
		return repo.Backend().Remove(backend.Lock, id.String())
	})
}
