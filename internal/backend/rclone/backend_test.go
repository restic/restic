package rclone_test

import (
	"os/exec"
	"testing"

	"github.com/restic/restic/internal/backend/rclone"
	"github.com/restic/restic/internal/backend/test"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func newTestSuite(t testing.TB) *test.Suite {
	dir, cleanup := rtest.TempDir(t)

	return &test.Suite{
		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (interface{}, error) {
			t.Logf("use backend at %v", dir)
			cfg := rclone.NewConfig()
			cfg.Remote = dir
			return cfg, nil
		},

		// CreateFn is a function that creates a temporary repository for the tests.
		Create: func(config interface{}) (restic.Backend, error) {
			t.Logf("Create()")
			cfg := config.(rclone.Config)
			be, err := rclone.Create(cfg)
			if e, ok := errors.Cause(err).(*exec.Error); ok && e.Err == exec.ErrNotFound {
				t.Skipf("program %q not found", e.Name)
				return nil, nil
			}
			return be, err
		},

		// OpenFn is a function that opens a previously created temporary repository.
		Open: func(config interface{}) (restic.Backend, error) {
			t.Logf("Open()")
			cfg := config.(rclone.Config)
			return rclone.Open(cfg)
		},

		// CleanupFn removes data created during the tests.
		Cleanup: func(config interface{}) error {
			t.Logf("cleanup dir %v", dir)
			cleanup()
			return nil
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
