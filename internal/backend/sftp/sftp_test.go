package sftp_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restic/restic/internal/backend/sftp"
	"github.com/restic/restic/internal/backend/test"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func findSFTPServerBinary() string {
	for _, dir := range strings.Split(rtest.TestSFTPPath, ":") {
		testpath := filepath.Join(dir, "sftp-server")
		_, err := os.Stat(testpath)
		if !errors.Is(err, os.ErrNotExist) {
			return testpath
		}
	}

	return ""
}

var sftpServer = findSFTPServerBinary()

func newTestSuite(t testing.TB) *test.Suite {
	return &test.Suite{
		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (interface{}, error) {
			dir, err := os.MkdirTemp(rtest.TestTempDir, "restic-test-sftp-")
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("create new backend at %v", dir)

			cfg := sftp.Config{
				Path:        dir,
				Command:     fmt.Sprintf("%q -e", sftpServer),
				Connections: 5,
			}
			return cfg, nil
		},

		// CreateFn is a function that creates a temporary repository for the tests.
		Create: func(config interface{}) (restic.Backend, error) {
			cfg := config.(sftp.Config)
			return sftp.Create(context.TODO(), cfg)
		},

		// OpenFn is a function that opens a previously created temporary repository.
		Open: func(config interface{}) (restic.Backend, error) {
			cfg := config.(sftp.Config)
			return sftp.Open(context.TODO(), cfg)
		},

		// CleanupFn removes data created during the tests.
		Cleanup: func(config interface{}) error {
			cfg := config.(sftp.Config)
			if !rtest.TestCleanupTempDirs {
				t.Logf("leaving test backend dir at %v", cfg.Path)
			}

			rtest.RemoveAll(t, cfg.Path)
			return nil
		},
	}
}

func TestBackendSFTP(t *testing.T) {
	defer func() {
		if t.Skipped() {
			rtest.SkipDisallowed(t, "restic/backend/sftp.TestBackendSFTP")
		}
	}()

	if sftpServer == "" {
		t.Skip("sftp server binary not found")
	}

	newTestSuite(t).RunTests(t)
}

func BenchmarkBackendSFTP(t *testing.B) {
	if sftpServer == "" {
		t.Skip("sftp server binary not found")
	}

	newTestSuite(t).RunBenchmarks(t)
}
