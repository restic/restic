package local_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/restic/restic/internal/backend/local"
	"github.com/restic/restic/internal/backend/test"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func newTestSuite(t testing.TB) *test.Suite {
	return &test.Suite{
		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (interface{}, error) {
			dir, err := os.MkdirTemp(rtest.TestTempDir, "restic-test-local-")
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("create new backend at %v", dir)

			cfg := local.Config{
				Path:        dir,
				Connections: 2,
			}
			return cfg, nil
		},

		// CreateFn is a function that creates a temporary repository for the tests.
		Create: func(config interface{}) (restic.Backend, error) {
			cfg := config.(local.Config)
			return local.Create(context.TODO(), cfg)
		},

		// OpenFn is a function that opens a previously created temporary repository.
		Open: func(config interface{}) (restic.Backend, error) {
			cfg := config.(local.Config)
			return local.Open(context.TODO(), cfg)
		},

		// CleanupFn removes data created during the tests.
		Cleanup: func(config interface{}) error {
			cfg := config.(local.Config)
			if !rtest.TestCleanupTempDirs {
				t.Logf("leaving test backend dir at %v", cfg.Path)
			}

			rtest.RemoveAll(t, cfg.Path)
			return nil
		},
	}
}

func TestBackend(t *testing.T) {
	newTestSuite(t).RunTests(t)
}

func BenchmarkBackend(t *testing.B) {
	newTestSuite(t).RunBenchmarks(t)
}

func readdirnames(t testing.TB, dir string) []string {
	f, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	entries, err := f.Readdirnames(-1)
	if err != nil {
		t.Fatal(err)
	}

	err = f.Close()
	if err != nil {
		t.Fatal(err)
	}

	return entries
}

func empty(t testing.TB, dir string) {
	entries := readdirnames(t, dir)
	if len(entries) != 0 {
		t.Fatalf("directory %v is not empty, contains: %v", dir, entries)
	}
}

func openclose(t testing.TB, dir string) {
	cfg := local.Config{Path: dir}

	be, err := local.Open(context.TODO(), cfg)
	if err != nil {
		t.Logf("Open returned error %v", err)
	}

	if be != nil {
		err = be.Close()
		if err != nil {
			t.Logf("Close returned error %v", err)
		}
	}
}

func mkdir(t testing.TB, dir string) {
	err := os.Mkdir(dir, 0700)
	if err != nil {
		t.Fatal(err)
	}
}

func removeAll(t testing.TB, dir string) {
	err := os.RemoveAll(dir)
	if err != nil {
		t.Fatal(err)
	}
}

func TestOpenNotExistingDirectory(t *testing.T) {
	dir := rtest.TempDir(t)

	// local.Open must not create any files dirs in the repo
	openclose(t, filepath.Join(dir, "repo"))
	empty(t, dir)

	openclose(t, dir)
	empty(t, dir)

	mkdir(t, filepath.Join(dir, "data"))
	openclose(t, dir)
	removeAll(t, filepath.Join(dir, "data"))
	empty(t, dir)
}
