package rclone_test

import (
	"context"
	"os/exec"
	"testing"

	"github.com/restic/restic/internal/backend/rclone"
	"github.com/restic/restic/internal/backend/test"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func newTestSuite(t testing.TB) *test.Suite[rclone.Config] {
	dir := rtest.TempDir(t)

	return &test.Suite[rclone.Config]{
		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (*rclone.Config, error) {
			t.Logf("use backend at %v", dir)
			cfg := rclone.NewConfig()
			cfg.Remote = dir
			return &cfg, nil
		},

		// CreateFn is a function that creates a temporary repository for the tests.
		Create: func(cfg rclone.Config) (restic.Backend, error) {
			t.Logf("Create()")
			be, err := rclone.Create(context.TODO(), cfg)
			var e *exec.Error
			if errors.As(err, &e) && e.Err == exec.ErrNotFound {
				t.Skipf("program %q not found", e.Name)
				return nil, nil
			}
			return be, err
		},

		// OpenFn is a function that opens a previously created temporary repository.
		Open: func(cfg rclone.Config) (restic.Backend, error) {
			t.Logf("Open()")
			return rclone.Open(context.TODO(), cfg, nil)
		},
	}
}

func TestBackendRclone(t *testing.T) {
	defer func() {
		if t.Skipped() {
			rtest.SkipDisallowed(t, "restic/backend/rclone.TestBackendRclone")
		}
	}()

	newTestSuite(t).RunTests(t)
}

func BenchmarkBackendREST(t *testing.B) {
	newTestSuite(t).RunBenchmarks(t)
}
