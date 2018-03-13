package rclone_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/restic/restic/internal/backend/rclone"
	"github.com/restic/restic/internal/backend/test"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

const rcloneConfig = `
[local]
type = local
`

func newTestSuite(t testing.TB) *test.Suite {
	dir, cleanup := rtest.TempDir(t)

	return &test.Suite{
		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (interface{}, error) {
			cfgfile := filepath.Join(dir, "rclone.conf")
			t.Logf("write rclone config to %v", cfgfile)
			err := ioutil.WriteFile(cfgfile, []byte(rcloneConfig), 0644)
			if err != nil {
				return nil, err
			}

			t.Logf("use backend at %v", dir)

			repodir := filepath.Join(dir, "repo")
			err = os.Mkdir(repodir, 0755)
			if err != nil {
				return nil, err
			}

			cfg := rclone.NewConfig()
			cfg.Program = fmt.Sprintf("rclone --config %q", cfgfile)
			cfg.Remote = "local:" + repodir
			return cfg, nil
		},

		// CreateFn is a function that creates a temporary repository for the tests.
		Create: func(config interface{}) (restic.Backend, error) {
			t.Logf("Create()")
			cfg := config.(rclone.Config)
			return rclone.Create(cfg)
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
