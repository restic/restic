package local_test

import (
	"io/ioutil"
	"restic"
	"testing"

	"restic/backend/local"
	"restic/backend/test"
	. "restic/test"
)

func TestBackend(t *testing.T) {
	suite := test.Suite{
		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (interface{}, error) {
			dir, err := ioutil.TempDir(TestTempDir, "restic-test-local-")
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("create new backend at %v", dir)

			cfg := local.Config{
				Path: dir,
			}
			return cfg, nil
		},

		// CreateFn is a function that creates a temporary repository for the tests.
		Create: func(config interface{}) (restic.Backend, error) {
			cfg := config.(local.Config)
			return local.Create(cfg)
		},

		// OpenFn is a function that opens a previously created temporary repository.
		Open: func(config interface{}) (restic.Backend, error) {
			cfg := config.(local.Config)
			return local.Open(cfg)
		},

		// CleanupFn removes data created during the tests.
		Cleanup: func(config interface{}) error {
			cfg := config.(local.Config)
			if !TestCleanupTempDirs {
				t.Logf("leaving test backend dir at %v", cfg.Path)
			}

			RemoveAll(t, cfg.Path)
			return nil
		},
	}

	suite.RunTests(t)
}
