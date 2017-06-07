package restic_test

import (
	"context"
	"os"
	"testing"
	"time"

	"restic"
	"restic/repository"
	. "restic/test"
)

func TestLock(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	lock, err := restic.NewLock(context.TODO(), repo)
	OK(t, err)

	OK(t, lock.Unlock())
}

func TestDoubleUnlock(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	lock, err := restic.NewLock(context.TODO(), repo)
	OK(t, err)

	OK(t, lock.Unlock())

	err = lock.Unlock()
	Assert(t, err != nil,
		"double unlock didn't return an error, got %v", err)
}

func TestMultipleLock(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	lock1, err := restic.NewLock(context.TODO(), repo)
	OK(t, err)

	lock2, err := restic.NewLock(context.TODO(), repo)
	OK(t, err)

	OK(t, lock1.Unlock())
	OK(t, lock2.Unlock())
}

func TestLockExclusive(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	elock, err := restic.NewExclusiveLock(context.TODO(), repo)
	OK(t, err)
	OK(t, elock.Unlock())
}

func TestLockOnExclusiveLockedRepo(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	elock, err := restic.NewExclusiveLock(context.TODO(), repo)
	OK(t, err)

	lock, err := restic.NewLock(context.TODO(), repo)
	Assert(t, err != nil,
		"create normal lock with exclusively locked repo didn't return an error")
	Assert(t, restic.IsAlreadyLocked(err),
		"create normal lock with exclusively locked repo didn't return the correct error")

	OK(t, lock.Unlock())
	OK(t, elock.Unlock())
}

func TestExclusiveLockOnLockedRepo(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	elock, err := restic.NewLock(context.TODO(), repo)
	OK(t, err)

	lock, err := restic.NewExclusiveLock(context.TODO(), repo)
	Assert(t, err != nil,
		"create normal lock with exclusively locked repo didn't return an error")
	Assert(t, restic.IsAlreadyLocked(err),
		"create normal lock with exclusively locked repo didn't return the correct error")

	OK(t, lock.Unlock())
	OK(t, elock.Unlock())
}

func createFakeLock(repo restic.Repository, t time.Time, pid int) (restic.ID, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return restic.ID{}, err
	}

	newLock := &restic.Lock{Time: t, PID: pid, Hostname: hostname}
	return repo.SaveJSONUnpacked(context.TODO(), restic.LockFile, &newLock)
}

func removeLock(repo restic.Repository, id restic.ID) error {
	h := restic.Handle{Type: restic.LockFile, Name: id.String()}
	return repo.Backend().Remove(context.TODO(), h)
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
	OK(t, err)

	otherHostname := "other-" + hostname

	for i, test := range staleLockTests {
		lock := restic.Lock{
			Time:     test.timestamp,
			PID:      test.pid,
			Hostname: hostname,
		}

		Assert(t, lock.Stale() == test.stale,
			"TestStaleLock: test %d failed: expected stale: %v, got %v",
			i, test.stale, !test.stale)

		lock.Hostname = otherHostname
		Assert(t, lock.Stale() == test.staleOnOtherHost,
			"TestStaleLock: test %d failed: expected staleOnOtherHost: %v, got %v",
			i, test.staleOnOtherHost, !test.staleOnOtherHost)
	}
}

func lockExists(repo restic.Repository, t testing.TB, id restic.ID) bool {
	h := restic.Handle{Type: restic.LockFile, Name: id.String()}
	exists, err := repo.Backend().Test(context.TODO(), h)
	OK(t, err)

	return exists
}

func TestLockWithStaleLock(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	id1, err := createFakeLock(repo, time.Now().Add(-time.Hour), os.Getpid())
	OK(t, err)

	id2, err := createFakeLock(repo, time.Now().Add(-time.Minute), os.Getpid())
	OK(t, err)

	id3, err := createFakeLock(repo, time.Now().Add(-time.Minute), os.Getpid()+500000)
	OK(t, err)

	OK(t, restic.RemoveStaleLocks(context.TODO(), repo))

	Assert(t, lockExists(repo, t, id1) == false,
		"stale lock still exists after RemoveStaleLocks was called")
	Assert(t, lockExists(repo, t, id2) == true,
		"non-stale lock was removed by RemoveStaleLocks")
	Assert(t, lockExists(repo, t, id3) == false,
		"stale lock still exists after RemoveStaleLocks was called")

	OK(t, removeLock(repo, id2))
}

func TestRemoveAllLocks(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	id1, err := createFakeLock(repo, time.Now().Add(-time.Hour), os.Getpid())
	OK(t, err)

	id2, err := createFakeLock(repo, time.Now().Add(-time.Minute), os.Getpid())
	OK(t, err)

	id3, err := createFakeLock(repo, time.Now().Add(-time.Minute), os.Getpid()+500000)
	OK(t, err)

	OK(t, restic.RemoveAllLocks(context.TODO(), repo))

	Assert(t, lockExists(repo, t, id1) == false,
		"lock still exists after RemoveAllLocks was called")
	Assert(t, lockExists(repo, t, id2) == false,
		"lock still exists after RemoveAllLocks was called")
	Assert(t, lockExists(repo, t, id3) == false,
		"lock still exists after RemoveAllLocks was called")
}

func TestLockRefresh(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	lock, err := restic.NewLock(context.TODO(), repo)
	OK(t, err)

	var lockID *restic.ID
	for id := range repo.List(context.TODO(), restic.LockFile) {
		if lockID != nil {
			t.Error("more than one lock found")
		}
		lockID = &id
	}

	OK(t, lock.Refresh(context.TODO()))

	var lockID2 *restic.ID
	for id := range repo.List(context.TODO(), restic.LockFile) {
		if lockID2 != nil {
			t.Error("more than one lock found")
		}
		lockID2 = &id
	}

	Assert(t, !lockID.Equal(*lockID2),
		"expected a new ID after lock refresh, got the same")
	OK(t, lock.Unlock())
}
