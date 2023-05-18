package sema_test

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/restic/restic/internal/backend/mock"
	"github.com/restic/restic/internal/backend/sema"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
	"golang.org/x/sync/errgroup"
)

func TestParameterValidationSave(t *testing.T) {
	m := mock.NewBackend()
	m.SaveFn = func(ctx context.Context, h restic.Handle, rd restic.RewindReader) error {
		return nil
	}
	be := sema.NewBackend(m)

	err := be.Save(context.TODO(), restic.Handle{}, nil)
	test.Assert(t, err != nil, "Save() with invalid handle did not return an error")
}

func TestParameterValidationLoad(t *testing.T) {
	m := mock.NewBackend()
	m.OpenReaderFn = func(ctx context.Context, h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
		return io.NopCloser(nil), nil
	}

	be := sema.NewBackend(m)
	nilCb := func(rd io.Reader) error { return nil }

	err := be.Load(context.TODO(), restic.Handle{}, 10, 0, nilCb)
	test.Assert(t, err != nil, "Load() with invalid handle did not return an error")

	h := restic.Handle{Type: restic.PackFile, Name: "foobar"}
	err = be.Load(context.TODO(), h, 10, -1, nilCb)
	test.Assert(t, err != nil, "Save() with negative offset did not return an error")
	err = be.Load(context.TODO(), h, -1, 0, nilCb)
	test.Assert(t, err != nil, "Save() with negative length did not return an error")
}

func TestParameterValidationStat(t *testing.T) {
	m := mock.NewBackend()
	m.StatFn = func(ctx context.Context, h restic.Handle) (restic.FileInfo, error) {
		return restic.FileInfo{}, nil
	}
	be := sema.NewBackend(m)

	_, err := be.Stat(context.TODO(), restic.Handle{})
	test.Assert(t, err != nil, "Stat() with invalid handle did not return an error")
}

func TestParameterValidationRemove(t *testing.T) {
	m := mock.NewBackend()
	m.RemoveFn = func(ctx context.Context, h restic.Handle) error {
		return nil
	}
	be := sema.NewBackend(m)

	err := be.Remove(context.TODO(), restic.Handle{})
	test.Assert(t, err != nil, "Remove() with invalid handle did not return an error")
}

func TestUnwrap(t *testing.T) {
	m := mock.NewBackend()
	be := sema.NewBackend(m)

	unwrapper := be.(restic.BackendUnwrapper)
	test.Assert(t, unwrapper.Unwrap() == m, "Unwrap() returned wrong backend")
}

func countingBlocker() (func(), func(int) int) {
	ctr := int64(0)
	blocker := make(chan struct{})

	wait := func() {
		// count how many goroutines were allowed by the semaphore
		atomic.AddInt64(&ctr, 1)
		// block until the test can retrieve the counter
		<-blocker
	}

	unblock := func(expected int) int {
		// give goroutines enough time to block
		var blocked int64
		for i := 0; i < 100 && blocked < int64(expected); i++ {
			time.Sleep(100 * time.Microsecond)
			blocked = atomic.LoadInt64(&ctr)
		}
		close(blocker)
		return int(blocked)
	}

	return wait, unblock
}

func concurrencyTester(t *testing.T, setup func(m *mock.Backend), handler func(be restic.Backend) func() error, unblock func(int) int, isUnlimited bool) {
	expectBlocked := int(2)
	workerCount := expectBlocked + 1

	m := mock.NewBackend()
	setup(m)
	m.ConnectionsFn = func() uint { return uint(expectBlocked) }
	be := sema.NewBackend(m)

	var wg errgroup.Group
	for i := 0; i < workerCount; i++ {
		wg.Go(handler(be))
	}

	if isUnlimited {
		expectBlocked = workerCount
	}
	blocked := unblock(expectBlocked)
	test.Assert(t, blocked == expectBlocked, "Unexpected number of goroutines blocked: %v", blocked)
	test.OK(t, wg.Wait())
}

func TestConcurrencyLimitSave(t *testing.T) {
	wait, unblock := countingBlocker()
	concurrencyTester(t, func(m *mock.Backend) {
		m.SaveFn = func(ctx context.Context, h restic.Handle, rd restic.RewindReader) error {
			wait()
			return nil
		}
	}, func(be restic.Backend) func() error {
		return func() error {
			h := restic.Handle{Type: restic.PackFile, Name: "foobar"}
			return be.Save(context.TODO(), h, nil)
		}
	}, unblock, false)
}

func TestConcurrencyLimitLoad(t *testing.T) {
	wait, unblock := countingBlocker()
	concurrencyTester(t, func(m *mock.Backend) {
		m.OpenReaderFn = func(ctx context.Context, h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
			wait()
			return io.NopCloser(nil), nil
		}
	}, func(be restic.Backend) func() error {
		return func() error {
			h := restic.Handle{Type: restic.PackFile, Name: "foobar"}
			nilCb := func(rd io.Reader) error { return nil }
			return be.Load(context.TODO(), h, 10, 0, nilCb)
		}
	}, unblock, false)
}

func TestConcurrencyLimitStat(t *testing.T) {
	wait, unblock := countingBlocker()
	concurrencyTester(t, func(m *mock.Backend) {
		m.StatFn = func(ctx context.Context, h restic.Handle) (restic.FileInfo, error) {
			wait()
			return restic.FileInfo{}, nil
		}
	}, func(be restic.Backend) func() error {
		return func() error {
			h := restic.Handle{Type: restic.PackFile, Name: "foobar"}
			_, err := be.Stat(context.TODO(), h)
			return err
		}
	}, unblock, false)
}

func TestConcurrencyLimitDelete(t *testing.T) {
	wait, unblock := countingBlocker()
	concurrencyTester(t, func(m *mock.Backend) {
		m.RemoveFn = func(ctx context.Context, h restic.Handle) error {
			wait()
			return nil
		}
	}, func(be restic.Backend) func() error {
		return func() error {
			h := restic.Handle{Type: restic.PackFile, Name: "foobar"}
			return be.Remove(context.TODO(), h)
		}
	}, unblock, false)
}

func TestConcurrencyUnlimitedLockSave(t *testing.T) {
	wait, unblock := countingBlocker()
	concurrencyTester(t, func(m *mock.Backend) {
		m.SaveFn = func(ctx context.Context, h restic.Handle, rd restic.RewindReader) error {
			wait()
			return nil
		}
	}, func(be restic.Backend) func() error {
		return func() error {
			h := restic.Handle{Type: restic.LockFile, Name: "foobar"}
			return be.Save(context.TODO(), h, nil)
		}
	}, unblock, true)
}
