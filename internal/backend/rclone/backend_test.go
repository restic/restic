package rclone_test

import (
	"os/exec"
	"testing"

	"github.com/restic/restic/internal/backend/rclone"
	"github.com/restic/restic/internal/backend/test"
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

		Factory: rclone.NewFactory(),
	}
}

func findRclone(t testing.TB) {
	// try to find a rclone binary
	_, err := exec.LookPath("rclone")
	if err != nil {
		t.Skip(err)
	}
}

func TestBackendRclone(t *testing.T) {
	defer func() {
		if t.Skipped() {
			rtest.SkipDisallowed(t, "restic/backend/rclone.TestBackendRclone")
		}
	}()

	findRclone(t)
	newTestSuite(t).RunTests(t)
}

func BenchmarkBackendREST(t *testing.B) {
	findRclone(t)
	newTestSuite(t).RunBenchmarks(t)
}
