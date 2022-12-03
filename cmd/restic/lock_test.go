package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func openTestRepo(t *testing.T, wrapper backendWrapper) (*repository.Repository, func(), *testEnvironment) {
	env, cleanup := withTestEnvironment(t)
	if wrapper != nil {
		env.gopts.backendTestHook = wrapper
	}
	testRunInit(t, env.gopts)

	repo, err := OpenRepository(context.TODO(), env.gopts)
	rtest.OK(t, err)
	return repo, cleanup, env
}

func checkedLockRepo(ctx context.Context, t *testing.T, repo restic.Repository) (*restic.Lock, context.Context) {
	lock, wrappedCtx, err := lockRepo(ctx, repo)
	rtest.OK(t, err)
	rtest.OK(t, wrappedCtx.Err())
	if lock.Stale() {
		t.Fatal("lock returned stale lock")
	}
	return lock, wrappedCtx
}

func TestLock(t *testing.T) {
	repo, cleanup, _ := openTestRepo(t, nil)
	defer cleanup()

	lock, wrappedCtx := checkedLockRepo(context.Background(), t, repo)
	unlockRepo(lock)
	if wrappedCtx.Err() == nil {
		t.Fatal("unlock did not cancel context")
	}
}

func TestLockCancel(t *testing.T) {
	repo, cleanup, _ := openTestRepo(t, nil)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	lock, wrappedCtx := checkedLockRepo(ctx, t, repo)
	cancel()
	if wrappedCtx.Err() == nil {
		t.Fatal("canceled parent context did not cancel context")
	}

	// unlockRepo should not crash
	unlockRepo(lock)
}

func TestLockUnlockAll(t *testing.T) {
	repo, cleanup, _ := openTestRepo(t, nil)
	defer cleanup()

	lock, wrappedCtx := checkedLockRepo(context.Background(), t, repo)
	_, err := unlockAll(0)
	rtest.OK(t, err)
	if wrappedCtx.Err() == nil {
		t.Fatal("canceled parent context did not cancel context")
	}

	// unlockRepo should not crash
	unlockRepo(lock)
}

func TestLockConflict(t *testing.T) {
	repo, cleanup, env := openTestRepo(t, nil)
	defer cleanup()
	repo2, err := OpenRepository(context.TODO(), env.gopts)
	rtest.OK(t, err)

	lock, _, err := lockRepoExclusive(context.Background(), repo)
	rtest.OK(t, err)
	defer unlockRepo(lock)
	_, _, err = lockRepo(context.Background(), repo2)
	if err == nil {
		t.Fatal("second lock should have failed")
	}
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
	repo, cleanup, _ := openTestRepo(t, func(r restic.Backend) (restic.Backend, error) {
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

	lock, wrappedCtx := checkedLockRepo(context.Background(), t, repo)

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
	return b.Backend.Save(ctx, h, rd)
}

func TestLockSuccessfulRefresh(t *testing.T) {
	repo, cleanup, _ := openTestRepo(t, func(r restic.Backend) (restic.Backend, error) {
		return &loggingBackend{
			Backend: r,
			t:       t,
		}, nil
	})
	defer cleanup()

	t.Logf("test for successful lock refresh %v", time.Now())
	// reduce locking intervals to be suitable for testing
	ri, rt := refreshInterval, refreshabilityTimeout
	refreshInterval = 40 * time.Millisecond
	refreshabilityTimeout = 200 * time.Millisecond
	defer func() {
		refreshInterval, refreshabilityTimeout = ri, rt
	}()

	lock, wrappedCtx := checkedLockRepo(context.Background(), t, repo)

	select {
	case <-wrappedCtx.Done():
		t.Fatal("lock refresh failed")
	case <-time.After(2 * refreshabilityTimeout):
		// expected lock refresh to work
	}
	// unlockRepo should not crash
	unlockRepo(lock)
}
