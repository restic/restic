package repository

import (
	"context"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/mem"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func TestLockFile(t *testing.T) {
	repo := TestRepository(t)
	TestSetLockTimeout(t, 5*time.Millisecond)

	lock, err := newLock(context.TODO(), &internalRepository{repo}, false)
	rtest.OK(t, err)

	rtest.OK(t, lock.unlock(context.TODO()))
}

func TestDoubleUnlock(t *testing.T) {
	repo := TestRepository(t)
	TestSetLockTimeout(t, 5*time.Millisecond)

	lock, err := newLock(context.TODO(), &internalRepository{repo}, false)
	rtest.OK(t, err)

	rtest.OK(t, lock.unlock(context.TODO()))

	err = lock.unlock(context.TODO())
	rtest.Assert(t, err != nil,
		"double unlock didn't return an error, got %v", err)
}

func TestMultipleLock(t *testing.T) {
	repo := TestRepository(t)
	TestSetLockTimeout(t, 5*time.Millisecond)

	lock1, err := newLock(context.TODO(), &internalRepository{repo}, false)
	rtest.OK(t, err)

	lock2, err := newLock(context.TODO(), &internalRepository{repo}, false)
	rtest.OK(t, err)

	rtest.OK(t, lock1.unlock(context.TODO()))
	rtest.OK(t, lock2.unlock(context.TODO()))
}

type failLockLoadingBackend struct {
	backend.Backend
}

func (be *failLockLoadingBackend) Load(ctx context.Context, h backend.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	if h.Type == backend.LockFile {
		return fmt.Errorf("error loading lock")
	}
	return be.Backend.Load(ctx, h, length, offset, fn)
}

func TestMultipleLockFailure(t *testing.T) {
	be := &failLockLoadingBackend{Backend: mem.New()}
	repo, _ := TestRepositoryWithBackend(t, be, 0, Options{})
	TestSetLockTimeout(t, 5*time.Millisecond)

	lock1, err := newLock(context.TODO(), &internalRepository{repo}, false)
	rtest.OK(t, err)

	_, err = newLock(context.TODO(), &internalRepository{repo}, false)
	rtest.Assert(t, err != nil, "unreadable lock file did not result in an error")

	rtest.OK(t, lock1.unlock(context.TODO()))
}

func TestLockExclusive(t *testing.T) {
	repo := TestRepository(t)

	elock, err := newLock(context.TODO(), &internalRepository{repo}, true)
	rtest.OK(t, err)
	rtest.OK(t, elock.unlock(context.TODO()))
}

func TestLockOnExclusiveLockedRepo(t *testing.T) {
	repo := TestRepository(t)
	TestSetLockTimeout(t, 5*time.Millisecond)

	elock, err := newLock(context.TODO(), &internalRepository{repo}, true)
	rtest.OK(t, err)

	lock, err := newLock(context.TODO(), &internalRepository{repo}, false)
	rtest.Assert(t, err != nil,
		"create normal lock with exclusively locked repo didn't return an error")
	rtest.Assert(t, IsAlreadyLocked(err),
		"create normal lock with exclusively locked repo didn't return the correct error")

	rtest.OK(t, lock.unlock(context.TODO()))
	rtest.OK(t, elock.unlock(context.TODO()))
}

func TestExclusiveLockOnLockedRepo(t *testing.T) {
	repo := TestRepository(t)
	TestSetLockTimeout(t, 5*time.Millisecond)

	elock, err := newLock(context.TODO(), &internalRepository{repo}, false)
	rtest.OK(t, err)

	lock, err := newLock(context.TODO(), &internalRepository{repo}, true)
	rtest.Assert(t, err != nil,
		"create normal lock with exclusively locked repo didn't return an error")
	rtest.Assert(t, IsAlreadyLocked(err),
		"create normal lock with exclusively locked repo didn't return the correct error")

	rtest.OK(t, lock.unlock(context.TODO()))
	rtest.OK(t, elock.unlock(context.TODO()))
}

var staleLockTests = []struct {
	timestamp        time.Time
	stale            bool
	staleOnOtherHost bool
	pid              int
}{
	{
		timestamp:        time.Now(),
		stale:            false,
		staleOnOtherHost: false,
		pid:              os.Getpid(),
	},
	{
		timestamp:        time.Now().Add(-time.Hour),
		stale:            true,
		staleOnOtherHost: true,
		pid:              os.Getpid(),
	},
	{
		timestamp:        time.Now().Add(3 * time.Minute),
		stale:            false,
		staleOnOtherHost: false,
		pid:              os.Getpid(),
	},
	{
		timestamp:        time.Now(),
		stale:            true,
		staleOnOtherHost: false,
		pid:              os.Getpid() + 500000,
	},
}

func TestLockStale(t *testing.T) {
	hostname, err := os.Hostname()
	rtest.OK(t, err)

	otherHostname := "other-" + hostname

	for i, test := range staleLockTests {
		lock := lockHandle{
			Lock: Lock{
				Time:     test.timestamp,
				PID:      test.pid,
				Hostname: hostname,
			},
		}

		rtest.Assert(t, lock.stale() == test.stale,
			"TestStaleLock: test %d failed: expected stale: %v, got %v",
			i, test.stale, !test.stale)

		lock.Hostname = otherHostname
		rtest.Assert(t, lock.stale() == test.staleOnOtherHost,
			"TestStaleLock: test %d failed: expected staleOnOtherHost: %v, got %v",
			i, test.staleOnOtherHost, !test.staleOnOtherHost)
	}
}

func checkSingleLock(t *testing.T, repo restic.Lister) restic.ID {
	t.Helper()
	var lockID *restic.ID
	err := repo.List(context.TODO(), restic.LockFile, func(id restic.ID, size int64) error {
		if lockID != nil {
			t.Error("more than one lock found")
		}
		lockID = &id
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if lockID == nil {
		t.Fatal("no lock found")
	}
	return *lockID
}

func testLockRefresh(t *testing.T, refresh func(lock *lockHandle) error) {
	repo := TestRepository(t)
	TestSetLockTimeout(t, 5*time.Millisecond)

	lock, err := newLock(context.TODO(), &internalRepository{repo}, false)
	rtest.OK(t, err)
	time0 := lock.Time

	lockID := checkSingleLock(t, repo)

	time.Sleep(time.Millisecond)
	rtest.OK(t, refresh(lock))

	lockID2 := checkSingleLock(t, repo)

	rtest.Assert(t, !lockID.Equal(lockID2),
		"expected a new ID after lock refresh, got the same")
	lock2, err := LoadLock(context.TODO(), repo, lockID2)
	rtest.OK(t, err)
	rtest.Assert(t, lock2.Time.After(time0),
		"expected a later timestamp after lock refresh")
	rtest.OK(t, lock.unlock(context.TODO()))
}

func TestLockRefresh(t *testing.T) {
	testLockRefresh(t, func(lock *lockHandle) error {
		return lock.refresh(context.TODO())
	})
}

func TestLockRefreshStale(t *testing.T) {
	testLockRefresh(t, func(lock *lockHandle) error {
		return lock.refreshStaleLock(context.TODO())
	})
}

func TestLockRefreshStaleMissing(t *testing.T) {
	repo, _, be := TestRepositoryWithVersion(t, 0)
	TestSetLockTimeout(t, 5*time.Millisecond)

	lock, err := newLock(context.TODO(), &internalRepository{repo}, false)
	rtest.OK(t, err)
	lockID := checkSingleLock(t, repo)

	// refresh must fail if lock was removed
	rtest.OK(t, be.Remove(context.TODO(), backend.Handle{Type: backend.LockFile, Name: lockID.String()}))
	time.Sleep(time.Millisecond)
	err = lock.refreshStaleLock(context.TODO())
	rtest.Assert(t, err == errRemovedLock, "unexpected error, expected %v, got %v", errRemovedLock, err)
}
