package main

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func testRunRebuildIndex(t testing.TB, gopts global.Options) {
	rtest.OK(t, withTermStatus(t, gopts, func(ctx context.Context, gopts global.Options) error {
		gopts.Quiet = true
		return runRebuildIndex(context.TODO(), RepairIndexOptions{}, gopts, gopts.Term)
	}))
}

func testRebuildIndex(t *testing.T, backendTestHook global.BackendWrapper) {
	repository.TestInjectKey(t, restic.TestParseID("b9883c60bed42db51be171ca52f055104b6ea7cfa2bc381c05b2b1f78231280c"), `{"mac":{"k":"maQ4ILA872XnDxHVEno94A==","r":"OptMBABwkgIsMQcHME8cBw=="},"encrypt":"janrR1efN7HyQ8kOZ9zhHixooZ/e+WelH0mT4v9WskQ="}`)
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	datafile := filepath.Join("..", "..", "internal", "checker", "testdata", "duplicate-packs-in-index-test-repo.tar.gz")
	rtest.SetupTarTestFixture(t, env.base, datafile)

	out, _, err := testRunCheckOutput(t, env.gopts, false)
	if !strings.Contains(out, "contained in several indexes") {
		t.Fatalf("did not find checker hint for packs in several indexes")
	}

	if err != nil {
		t.Fatalf("expected no error from checker for test repository, got %v", err)
	}

	if !strings.Contains(out, "restic repair index") {
		t.Fatalf("did not find hint for repair index command")
	}

	env.gopts.BackendTestHook = backendTestHook
	testRunRebuildIndex(t, env.gopts)

	env.gopts.BackendTestHook = nil
	out, _, err = testRunCheckOutput(t, env.gopts, false)
	if len(out) != 0 {
		t.Fatalf("expected no output from the checker, got: %v", out)
	}

	if err != nil {
		t.Fatalf("expected no error from checker after repair index, got: %v", err)
	}
}

func TestRebuildIndex(t *testing.T) {
	testRebuildIndex(t, nil)
}

// indexErrorBackend modifies the first index after reading.
type indexErrorBackend struct {
	backend.Backend
	lock     sync.Mutex
	hasErred bool
}

func (b *indexErrorBackend) Load(ctx context.Context, h backend.Handle, length int, offset int64, consumer func(rd io.Reader) error) error {
	return b.Backend.Load(ctx, h, length, offset, func(rd io.Reader) error {
		// protect hasErred
		b.lock.Lock()
		defer b.lock.Unlock()
		if !b.hasErred && h.Type == backend.IndexFile {
			b.hasErred = true
			return consumer(errorReadCloser{rd})
		}
		return consumer(rd)
	})
}

type errorReadCloser struct {
	io.Reader
}

func (erd errorReadCloser) Read(p []byte) (int, error) {
	n, err := erd.Reader.Read(p)
	if n > 0 {
		p[0] ^= 1
	}
	return n, err
}

func TestRebuildIndexDamage(t *testing.T) {
	testRebuildIndex(t, func(r backend.Backend) (backend.Backend, error) {
		return &indexErrorBackend{
			Backend: r,
		}, nil
	})
}

type appendOnlyBackend struct {
	backend.Backend
}

// called via repo.Backend().Remove()
func (b *appendOnlyBackend) Remove(_ context.Context, h backend.Handle) error {
	return errors.Errorf("Failed to remove %v", h)
}

func TestRebuildIndexFailsOnAppendOnly(t *testing.T) {
	repository.TestInjectKey(t, restic.TestParseID("b9883c60bed42db51be171ca52f055104b6ea7cfa2bc381c05b2b1f78231280c"), `{"mac":{"k":"maQ4ILA872XnDxHVEno94A==","r":"OptMBABwkgIsMQcHME8cBw=="},"encrypt":"janrR1efN7HyQ8kOZ9zhHixooZ/e+WelH0mT4v9WskQ="}`)
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	datafile := filepath.Join("..", "..", "internal", "checker", "testdata", "duplicate-packs-in-index-test-repo.tar.gz")
	rtest.SetupTarTestFixture(t, env.base, datafile)

	env.gopts.BackendTestHook = func(r backend.Backend) (backend.Backend, error) {
		return &appendOnlyBackend{r}, nil
	}
	err := withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		gopts.Quiet = true
		return runRebuildIndex(context.TODO(), RepairIndexOptions{}, gopts, gopts.Term)
	})

	if err == nil {
		t.Error("expected rebuildIndex to fail")
	}
	t.Log(err)
}
