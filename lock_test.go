package restic_test

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/repository"
	. "github.com/restic/restic/test"
)

func TestLock(t *testing.T) {
	repo := SetupRepo()
	defer TeardownRepo(repo)

	lock, err := restic.NewLock(repo)
	OK(t, err)

	OK(t, lock.Unlock())
}

func TestDoubleUnlock(t *testing.T) {
	repo := SetupRepo()
	defer TeardownRepo(repo)

	lock, err := restic.NewLock(repo)
	OK(t, err)

	OK(t, lock.Unlock())

	err = lock.Unlock()
	Assert(t, err != nil,
		"double unlock didn't return an error, got %v", err)
}

func TestMultipleLock(t *testing.T) {
	repo := SetupRepo()
	defer TeardownRepo(repo)

	lock1, err := restic.NewLock(repo)
	OK(t, err)

	lock2, err := restic.NewLock(repo)
	OK(t, err)

	OK(t, lock1.Unlock())
	OK(t, lock2.Unlock())
}

func TestLockExclusive(t *testing.T) {
	repo := SetupRepo()
	defer TeardownRepo(repo)

	elock, err := restic.NewExclusiveLock(repo)
	OK(t, err)
	OK(t, elock.Unlock())
}

func TestLockOnExclusiveLockedRepo(t *testing.T) {
	repo := SetupRepo()
	defer TeardownRepo(repo)

	elock, err := restic.NewExclusiveLock(repo)
	OK(t, err)

	lock, err := restic.NewLock(repo)
	Assert(t, err == restic.ErrAlreadyLocked,
		"create normal lock with exclusively locked repo didn't return an error")

	OK(t, lock.Unlock())
	OK(t, elock.Unlock())
}

func TestExclusiveLockOnLockedRepo(t *testing.T) {
	repo := SetupRepo()
	defer TeardownRepo(repo)

	elock, err := restic.NewLock(repo)
	OK(t, err)

	lock, err := restic.NewExclusiveLock(repo)
	Assert(t, err == restic.ErrAlreadyLocked,
		"create exclusive lock with locked repo didn't return an error")

	OK(t, lock.Unlock())
	OK(t, elock.Unlock())
}

func createFakeLock(repo *repository.Repository, t time.Time, pid int) (backend.ID, error) {
	newLock := &restic.Lock{Time: t, PID: pid}
	return repo.SaveJSONUnpacked(backend.Lock, &newLock)
}

func removeLock(repo *repository.Repository, id backend.ID) error {
	return repo.Backend().Remove(backend.Lock, id.String())
}

var staleLockTests = []struct {
	timestamp time.Time
	stale     bool
	pid       int
}{
	{
		timestamp: time.Now(),
		stale:     false,
		pid:       os.Getpid(),
	},
	{
		timestamp: time.Now().Add(-time.Hour),
		stale:     true,
		pid:       os.Getpid(),
	},
	{
		timestamp: time.Now().Add(3 * time.Minute),
		stale:     false,
		pid:       os.Getpid(),
	},
	{
		timestamp: time.Now(),
		stale:     true,
		pid:       os.Getpid() + 500,
	},
}

func TestLockStale(t *testing.T) {
	for i, test := range staleLockTests {
		lock := restic.Lock{
			Time: test.timestamp,
			PID:  test.pid,
		}

		Assert(t, lock.Stale() == test.stale,
			"TestStaleLock: test %d failed: expected stale: %v, got %v",
			i, test.stale, !test.stale)
	}
}

func lockExists(repo *repository.Repository, t testing.TB, id backend.ID) bool {
	exists, err := repo.Backend().Test(backend.Lock, id.String())
	OK(t, err)

	return exists
}

func TestLockWithStaleLock(t *testing.T) {
	repo := SetupRepo()
	defer TeardownRepo(repo)

	id1, err := createFakeLock(repo, time.Now().Add(-time.Hour), os.Getpid())
	OK(t, err)

	id2, err := createFakeLock(repo, time.Now().Add(-time.Minute), os.Getpid())
	OK(t, err)

	id3, err := createFakeLock(repo, time.Now().Add(-time.Minute), os.Getpid()+500)
	OK(t, err)

	OK(t, restic.RemoveStaleLocks(repo))

	Assert(t, lockExists(repo, t, id1) == false,
		"stale lock still exists after RemoveStaleLocks was called")
	Assert(t, lockExists(repo, t, id2) == true,
		"non-stale lock was removed by RemoveStaleLocks")
	Assert(t, lockExists(repo, t, id3) == false,
		"stale lock still exists after RemoveStaleLocks was called")

	OK(t, removeLock(repo, id2))
}

func TestLockConflictingExclusiveLocks(t *testing.T) {
	repo := SetupRepo()
	defer TeardownRepo(repo)

	for _, jobs := range []int{5, 23, 200} {
		var wg sync.WaitGroup
		errch := make(chan error, jobs)

		f := func() {
			defer wg.Done()

			lock, err := restic.NewExclusiveLock(repo)
			errch <- err
			OK(t, lock.Unlock())
		}

		for i := 0; i < jobs; i++ {
			wg.Add(1)
			go f()
		}

		errors := 0
		for i := 0; i < jobs; i++ {
			err := <-errch
			if err != nil {
				errors++
			}
		}

		wg.Wait()

		Assert(t, errors == jobs-1,
			"Expected %d errors, got %d", jobs-1, errors)
	}
}

func TestLockConflictingLocks(t *testing.T) {
	repo := SetupRepo()
	defer TeardownRepo(repo)

	var wg sync.WaitGroup

	errch := make(chan error, 2)

	wg.Add(2)

	go func() {
		defer wg.Done()

		lock, err := restic.NewExclusiveLock(repo)
		errch <- err
		OK(t, lock.Unlock())
	}()

	go func() {
		defer wg.Done()

		lock, err := restic.NewLock(repo)
		errch <- err
		OK(t, lock.Unlock())
	}()

	errors := 0
	for i := 0; i < 2; i++ {
		err := <-errch
		if err != nil {
			errors++
		}
	}

	wg.Wait()

	Assert(t, errors == 1,
		"Expected exactly one errors, got %d", errors)
}
