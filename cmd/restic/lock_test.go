package main

import (
	"context"
	"github.com/restic/restic/internal/repository"
	"strings"
	"testing"
	"time"

	"github.com/restic/restic/internal/test"
)

func TestLockWaitTimeout(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	elock, err := lockRepoExclusive(context.TODO(), repo.(*repository.Repository))
	test.OK(t, err)

	wait := 2 * time.Second
	oldWaitLock := globalOptions.WaitLock
	globalOptions.WaitLock = wait
	start := time.Now()
	lock, err := lockRepo(context.TODO(), repo.(*repository.Repository))
	duration := time.Since(start)
	globalOptions.WaitLock = oldWaitLock
	test.Assert(t, err != nil,
		"create normal lock with exclusively locked repo didn't return an error")
	test.Assert(t, strings.Contains(err.Error(), "repository is already locked exclusively"),
		"create normal lock with exclusively locked repo didn't return the correct error")
	test.Assert(t, duration >= wait,
		"create normal lock with exclusively locked repo didn't wait for the specified timeout")

	test.OK(t, lock.Unlock())
	test.OK(t, elock.Unlock())
}

func TestLockWaitSuccess(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	elock, err := lockRepoExclusive(context.TODO(), repo.(*repository.Repository))
	test.OK(t, err)

	wait := 2 * time.Second
	time.AfterFunc(wait/2, func() {
		test.OK(t, elock.Unlock())
	})

	oldWaitLock := globalOptions.WaitLock
	globalOptions.WaitLock = wait
	lock, err := lockRepo(context.TODO(), repo.(*repository.Repository))
	globalOptions.WaitLock = oldWaitLock
	test.OK(t, err)

	test.OK(t, lock.Unlock())
}
