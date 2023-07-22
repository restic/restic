package main

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/restic/restic/internal/backend/location"
	"github.com/restic/restic/internal/backend/mem"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
)

func openLockTestRepo(t *testing.T, wrapper backendWrapper) (*repository.Repository, func(), *testEnvironment) {
	env, cleanup := withTestEnvironment(t)

	reg := location.NewRegistry()
	reg.Register(mem.NewFactory())
	env.gopts.backends = reg
	env.gopts.Repo = "mem:"

	if wrapper != nil {
		env.gopts.backendTestHook = wrapper
	}
	testRunInit(t, env.gopts)

	repo, err := OpenRepository(context.TODO(), env.gopts)
	test.OK(t, err)
	return repo, cleanup, env
}

func checkedLockRepo(ctx context.Context, t *testing.T, repo restic.Repository, env *testEnvironment) (*restic.Lock, context.Context) {
	lock, wrappedCtx, err := lockRepo(ctx, repo, env.gopts.RetryLock, env.gopts.JSON)
	test.OK(t, err)
	test.OK(t, wrappedCtx.Err())
	if lock.Stale() {
		t.Fatal("lock returned stale lock")
	}
	return lock, wrappedCtx
}

func TestLock(t *testing.T) {
	repo, cleanup, env := openLockTestRepo(t, nil)
	defer cleanup()

	lock, wrappedCtx := checkedLockRepo(context.Background(), t, repo, env)
	unlockRepo(lock)
	if wrappedCtx.Err() == nil {
		t.Fatal("unlock did not cancel context")
	}
}

func TestLockCancel(t *testing.T) {
	repo, cleanup, env := openLockTestRepo(t, nil)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	lock, wrappedCtx := checkedLockRepo(ctx, t, repo, env)
	cancel()
	if wrappedCtx.Err() == nil {
		t.Fatal("canceled parent context did not cancel context")
	}

	// unlockRepo should not crash
	unlockRepo(lock)
}

func TestLockUnlockAll(t *testing.T) {
	repo, cleanup, env := openLockTestRepo(t, nil)
	defer cleanup()

	lock, wrappedCtx := checkedLockRepo(context.Background(), t, repo, env)
	_, err := unlockAll(0)
	test.OK(t, err)
	if wrappedCtx.Err() == nil {
		t.Fatal("canceled parent context did not cancel context")
	}

	// unlockRepo should not crash
	unlockRepo(lock)
}

func TestLockConflict(t *testing.T) {
	repo, cleanup, env := openLockTestRepo(t, nil)
	defer cleanup()
	repo2, err := OpenRepository(context.TODO(), env.gopts)
	test.OK(t, err)

	lock, _, err := lockRepoExclusive(context.Background(), repo, env.gopts.RetryLock, env.gopts.JSON)
	test.OK(t, err)
	defer unlockRepo(lock)
	_, _, err = lockRepo(context.Background(), repo2, env.gopts.RetryLock, env.gopts.JSON)
	if err == nil {
		t.Fatal("second lock should have failed")
	}
	test.Assert(t, restic.IsAlreadyLocked(err), "unexpected error %v", err)
}

type writeOnceBackend struct {
	restic.Backend
	written bool
}

func (b *writeOnceBackend) Save(ctx context.Context, h restic.Handle, rd restic.RewindReader) error {
	if b.written {
		return fmt.Errorf("fail after first write")
	}
	b.written = true
	return b.Backend.Save(ctx, h, rd)
}

func TestLockFailedRefresh(t *testing.T) {
	repo, cleanup, env := openLockTestRepo(t, func(r restic.Backend) (restic.Backend, error) {
		return &writeOnceBackend{Backend: r}, nil
	})
	defer cleanup()

	// reduce locking intervals to be suitable for testing
	ri, rt := refreshInterval, refreshabilityTimeout
	refreshInterval = 20 * time.Millisecond
	refreshabilityTimeout = 100 * time.Millisecond
	defer func() {
		refreshInterval, refreshabilityTimeout = ri, rt
	}()

	lock, wrappedCtx := checkedLockRepo(context.Background(), t, repo, env)

	select {
	case <-wrappedCtx.Done():
		// expected lock refresh failure
	case <-time.After(time.Second):
		t.Fatal("failed lock refresh did not cause context cancellation")
	}
	// unlockRepo should not crash
	unlockRepo(lock)
}

type loggingBackend struct {
	restic.Backend
	t *testing.T
}

func (b *loggingBackend) Save(ctx context.Context, h restic.Handle, rd restic.RewindReader) error {
	b.t.Logf("save %v @ %v", h, time.Now())
	err := b.Backend.Save(ctx, h, rd)
	b.t.Logf("save finished %v @ %v", h, time.Now())
	return err
}

func TestLockSuccessfulRefresh(t *testing.T) {
	repo, cleanup, env := openLockTestRepo(t, func(r restic.Backend) (restic.Backend, error) {
		return &loggingBackend{
			Backend: r,
			t:       t,
		}, nil
	})
	defer cleanup()

	t.Logf("test for successful lock refresh %v", time.Now())
	// reduce locking intervals to be suitable for testing
	ri, rt := refreshInterval, refreshabilityTimeout
	refreshInterval = 60 * time.Millisecond
	refreshabilityTimeout = 500 * time.Millisecond
	defer func() {
		refreshInterval, refreshabilityTimeout = ri, rt
	}()

	lock, wrappedCtx := checkedLockRepo(context.Background(), t, repo, env)

	select {
	case <-wrappedCtx.Done():
		// don't call t.Fatal to allow the lock to be properly cleaned up
		t.Error("lock refresh failed", time.Now())

		// Dump full stacktrace
		buf := make([]byte, 1024*1024)
		n := runtime.Stack(buf, true)
		buf = buf[:n]
		t.Log(string(buf))

	case <-time.After(2 * refreshabilityTimeout):
		// expected lock refresh to work
	}
	// unlockRepo should not crash
	unlockRepo(lock)
}

type slowBackend struct {
	restic.Backend
	m     sync.Mutex
	sleep time.Duration
}

func (b *slowBackend) Save(ctx context.Context, h restic.Handle, rd restic.RewindReader) error {
	b.m.Lock()
	sleep := b.sleep
	b.m.Unlock()
	time.Sleep(sleep)
	return b.Backend.Save(ctx, h, rd)
}

func TestLockSuccessfulStaleRefresh(t *testing.T) {
	var sb *slowBackend
	repo, cleanup, env := openLockTestRepo(t, func(r restic.Backend) (restic.Backend, error) {
		sb = &slowBackend{Backend: r}
		return sb, nil
	})
	defer cleanup()

	t.Logf("test for successful lock refresh %v", time.Now())
	// reduce locking intervals to be suitable for testing
	ri, rt := refreshInterval, refreshabilityTimeout
	refreshInterval = 10 * time.Millisecond
	refreshabilityTimeout = 50 * time.Millisecond
	defer func() {
		refreshInterval, refreshabilityTimeout = ri, rt
	}()

	lock, wrappedCtx := checkedLockRepo(context.Background(), t, repo, env)
	// delay lock refreshing long enough that the lock would expire
	sb.m.Lock()
	sb.sleep = refreshabilityTimeout + refreshInterval
	sb.m.Unlock()

	select {
	case <-wrappedCtx.Done():
		// don't call t.Fatal to allow the lock to be properly cleaned up
		t.Error("lock refresh failed", time.Now())

	case <-time.After(refreshabilityTimeout):
	}
	// reset slow backend
	sb.m.Lock()
	sb.sleep = 0
	sb.m.Unlock()
	debug.Log("normal lock period has expired")

	select {
	case <-wrappedCtx.Done():
		// don't call t.Fatal to allow the lock to be properly cleaned up
		t.Error("lock refresh failed", time.Now())

	case <-time.After(3 * refreshabilityTimeout):
		// expected lock refresh to work
	}

	// unlockRepo should not crash
	unlockRepo(lock)
}

func TestLockWaitTimeout(t *testing.T) {
	repo, cleanup, env := openLockTestRepo(t, nil)
	defer cleanup()

	elock, _, err := lockRepoExclusive(context.TODO(), repo, env.gopts.RetryLock, env.gopts.JSON)
	test.OK(t, err)

	retryLock := 200 * time.Millisecond

	start := time.Now()
	lock, _, err := lockRepo(context.TODO(), repo, retryLock, env.gopts.JSON)
	duration := time.Since(start)

	test.Assert(t, err != nil,
		"create normal lock with exclusively locked repo didn't return an error")
	test.Assert(t, strings.Contains(err.Error(), "repository is already locked exclusively"),
		"create normal lock with exclusively locked repo didn't return the correct error")
	test.Assert(t, retryLock <= duration && duration < retryLock*3/2,
		"create normal lock with exclusively locked repo didn't wait for the specified timeout")

	test.OK(t, lock.Unlock())
	test.OK(t, elock.Unlock())
}

func TestLockWaitCancel(t *testing.T) {
	repo, cleanup, env := openLockTestRepo(t, nil)
	defer cleanup()

	elock, _, err := lockRepoExclusive(context.TODO(), repo, env.gopts.RetryLock, env.gopts.JSON)
	test.OK(t, err)

	retryLock := 200 * time.Millisecond
	cancelAfter := 40 * time.Millisecond

	start := time.Now()
	ctx, cancel := context.WithCancel(context.TODO())
	time.AfterFunc(cancelAfter, cancel)

	lock, _, err := lockRepo(ctx, repo, retryLock, env.gopts.JSON)
	duration := time.Since(start)

	test.Assert(t, err != nil,
		"create normal lock with exclusively locked repo didn't return an error")
	test.Assert(t, strings.Contains(err.Error(), "context canceled"),
		"create normal lock with exclusively locked repo didn't return the correct error")
	test.Assert(t, cancelAfter <= duration && duration < retryLock-10*time.Millisecond,
		"create normal lock with exclusively locked repo didn't return in time, duration %v", duration)

	test.OK(t, lock.Unlock())
	test.OK(t, elock.Unlock())
}

func TestLockWaitSuccess(t *testing.T) {
	repo, cleanup, env := openLockTestRepo(t, nil)
	defer cleanup()

	elock, _, err := lockRepoExclusive(context.TODO(), repo, env.gopts.RetryLock, env.gopts.JSON)
	test.OK(t, err)

	retryLock := 200 * time.Millisecond
	unlockAfter := 40 * time.Millisecond

	time.AfterFunc(unlockAfter, func() {
		test.OK(t, elock.Unlock())
	})

	lock, _, err := lockRepo(context.TODO(), repo, retryLock, env.gopts.JSON)
	test.OK(t, err)

	test.OK(t, lock.Unlock())
}
