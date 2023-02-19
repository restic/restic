package restic_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/restic/restic/internal/backend/mem"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func TestLock(t *testing.T) {
	repo := repository.TestRepository(t)

	lock, err := restic.NewLock(context.TODO(), repo)
	rtest.OK(t, err)

	rtest.OK(t, lock.Unlock())
}

func TestDoubleUnlock(t *testing.T) {
	repo := repository.TestRepository(t)

	lock, err := restic.NewLock(context.TODO(), repo)
	rtest.OK(t, err)

	rtest.OK(t, lock.Unlock())

	err = lock.Unlock()
	rtest.Assert(t, err != nil,
		"double unlock didn't return an error, got %v", err)
}

func TestMultipleLock(t *testing.T) {
	repo := repository.TestRepository(t)

	lock1, err := restic.NewLock(context.TODO(), repo)
	rtest.OK(t, err)

	lock2, err := restic.NewLock(context.TODO(), repo)
	rtest.OK(t, err)

	rtest.OK(t, lock1.Unlock())
	rtest.OK(t, lock2.Unlock())
}

type failLockLoadingBackend struct {
	restic.Backend
}

func (be *failLockLoadingBackend) Load(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	if h.Type == restic.LockFile {
		return fmt.Errorf("error loading lock")
	}
	return be.Backend.Load(ctx, h, length, offset, fn)
}

func TestMultipleLockFailure(t *testing.T) {
	be := &failLockLoadingBackend{Backend: mem.New()}
	repo := repository.TestRepositoryWithBackend(t, be, 0)

	lock1, err := restic.NewLock(context.TODO(), repo)
	rtest.OK(t, err)

	_, err = restic.NewLock(context.TODO(), repo)
	rtest.Assert(t, err != nil, "unreadable lock file did not result in an error")

	rtest.OK(t, lock1.Unlock())
}

func TestLockExclusive(t *testing.T) {
	repo := repository.TestRepository(t)

	elock, err := restic.NewExclusiveLock(context.TODO(), repo)
	rtest.OK(t, err)
	rtest.OK(t, elock.Unlock())
}

func TestLockOnExclusiveLockedRepo(t *testing.T) {
	repo := repository.TestRepository(t)

	elock, err := restic.NewExclusiveLock(context.TODO(), repo)
	rtest.OK(t, err)

	lock, err := restic.NewLock(context.TODO(), repo)
	rtest.Assert(t, err != nil,
		"create normal lock with exclusively locked repo didn't return an error")
	rtest.Assert(t, restic.IsAlreadyLocked(err),
		"create normal lock with exclusively locked repo didn't return the correct error")

	rtest.OK(t, lock.Unlock())
	rtest.OK(t, elock.Unlock())
}

func TestExclusiveLockOnLockedRepo(t *testing.T) {
	repo := repository.TestRepository(t)

	elock, err := restic.NewLock(context.TODO(), repo)
	rtest.OK(t, err)

	lock, err := restic.NewExclusiveLock(context.TODO(), repo)
	rtest.Assert(t, err != nil,
		"create normal lock with exclusively locked repo didn't return an error")
	rtest.Assert(t, restic.IsAlreadyLocked(err),
		"create normal lock with exclusively locked repo didn't return the correct error")

	rtest.OK(t, lock.Unlock())
	rtest.OK(t, elock.Unlock())
}

func createFakeLock(repo restic.Repository, t time.Time, pid int) (restic.ID, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return restic.ID{}, err
	}

	newLock := &restic.Lock{Time: t, PID: pid, Hostname: hostname}
	return restic.SaveJSONUnpacked(context.TODO(), repo, restic.LockFile, &newLock)
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
	rtest.OK(t, err)

	otherHostname := "other-" + hostname

	for i, test := range staleLockTests {
		lock := restic.Lock{
			Time:     test.timestamp,
			PID:      test.pid,
			Hostname: hostname,
		}

		rtest.Assert(t, lock.Stale() == test.stale,
			"TestStaleLock: test %d failed: expected stale: %v, got %v",
			i, test.stale, !test.stale)

		lock.Hostname = otherHostname
		rtest.Assert(t, lock.Stale() == test.staleOnOtherHost,
			"TestStaleLock: test %d failed: expected staleOnOtherHost: %v, got %v",
			i, test.staleOnOtherHost, !test.staleOnOtherHost)
	}
}

func lockExists(repo restic.Repository, t testing.TB, id restic.ID) bool {
	h := restic.Handle{Type: restic.LockFile, Name: id.String()}
	_, err := repo.Backend().Stat(context.TODO(), h)
	if err != nil && !repo.Backend().IsNotExist(err) {
		t.Fatal(err)
	}
	return err == nil
}

func TestLockWithStaleLock(t *testing.T) {
	repo := repository.TestRepository(t)

	id1, err := createFakeLock(repo, time.Now().Add(-time.Hour), os.Getpid())
	rtest.OK(t, err)

	id2, err := createFakeLock(repo, time.Now().Add(-time.Minute), os.Getpid())
	rtest.OK(t, err)

	id3, err := createFakeLock(repo, time.Now().Add(-time.Minute), os.Getpid()+500000)
	rtest.OK(t, err)

	processed, err := restic.RemoveStaleLocks(context.TODO(), repo)
	rtest.OK(t, err)

	rtest.Assert(t, lockExists(repo, t, id1) == false,
		"stale lock still exists after RemoveStaleLocks was called")
	rtest.Assert(t, lockExists(repo, t, id2) == true,
		"non-stale lock was removed by RemoveStaleLocks")
	rtest.Assert(t, lockExists(repo, t, id3) == false,
		"stale lock still exists after RemoveStaleLocks was called")
	rtest.Assert(t, processed == 2,
		"number of locks removed does not match: expected %d, got %d",
		2, processed)

	rtest.OK(t, removeLock(repo, id2))
}

func TestRemoveAllLocks(t *testing.T) {
	repo := repository.TestRepository(t)

	id1, err := createFakeLock(repo, time.Now().Add(-time.Hour), os.Getpid())
	rtest.OK(t, err)

	id2, err := createFakeLock(repo, time.Now().Add(-time.Minute), os.Getpid())
	rtest.OK(t, err)

	id3, err := createFakeLock(repo, time.Now().Add(-time.Minute), os.Getpid()+500000)
	rtest.OK(t, err)

	processed, err := restic.RemoveAllLocks(context.TODO(), repo)
	rtest.OK(t, err)

	rtest.Assert(t, lockExists(repo, t, id1) == false,
		"lock still exists after RemoveAllLocks was called")
	rtest.Assert(t, lockExists(repo, t, id2) == false,
		"lock still exists after RemoveAllLocks was called")
	rtest.Assert(t, lockExists(repo, t, id3) == false,
		"lock still exists after RemoveAllLocks was called")
	rtest.Assert(t, processed == 3,
		"number of locks removed does not match: expected %d, got %d",
		3, processed)
}

func TestLockRefresh(t *testing.T) {
	repo := repository.TestRepository(t)

	lock, err := restic.NewLock(context.TODO(), repo)
	rtest.OK(t, err)
	time0 := lock.Time

	var lockID *restic.ID
	err = repo.List(context.TODO(), restic.LockFile, func(id restic.ID, size int64) error {
		if lockID != nil {
			t.Error("more than one lock found")
		}
		lockID = &id
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Millisecond)
	rtest.OK(t, lock.Refresh(context.TODO()))

	var lockID2 *restic.ID
	err = repo.List(context.TODO(), restic.LockFile, func(id restic.ID, size int64) error {
		if lockID2 != nil {
			t.Error("more than one lock found")
		}
		lockID2 = &id
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	rtest.Assert(t, !lockID.Equal(*lockID2),
		"expected a new ID after lock refresh, got the same")
	lock2, err := restic.LoadLock(context.TODO(), repo, *lockID2)
	rtest.OK(t, err)
	rtest.Assert(t, lock2.Time.After(time0),
		"expected a later timestamp after lock refresh")
	rtest.OK(t, lock.Unlock())
}
