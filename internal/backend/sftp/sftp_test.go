package sftp_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restic/restic/internal/backend/sftp"
	"github.com/restic/restic/internal/backend/test"
	"github.com/restic/restic/internal/errors"
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

func newTestSuite(t testing.TB) *test.Suite[sftp.Config] {
	return &test.Suite[sftp.Config]{
		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (*sftp.Config, error) {
			dir := rtest.TempDir(t)
			t.Logf("create new backend at %v", dir)

			cfg := &sftp.Config{
				Path:        dir,
				Command:     fmt.Sprintf("%q -e", sftpServer),
				Connections: 5,
			}
			return cfg, nil
		},

		Factory: sftp.NewFactory(),
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
