package sftp_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"restic"
	"restic/backend/sftp"
	"restic/backend/test"
	"restic/errors"
	"strings"
	"testing"

	. "restic/test"
)

func findSFTPServerBinary() string {
	for _, dir := range strings.Split(TestSFTPPath, ":") {
		testpath := filepath.Join(dir, "sftp-server")
		_, err := os.Stat(testpath)
		if !os.IsNotExist(errors.Cause(err)) {
			return testpath
		}
	}

	return ""
}

var sftpServer = findSFTPServerBinary()

func TestBackend(t *testing.T) {
	if sftpServer == "" {
		t.Skip("sftp server binary not found")
	}

	suite := test.Suite{
		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (interface{}, error) {
			dir, err := ioutil.TempDir(TestTempDir, "restic-test-sftp-")
			if err != nil {
				t.Fatal(err)
			}

			t.Logf("create new backend at %v", dir)

			cfg := sftp.Config{
				Path:    dir,
				Command: fmt.Sprintf("%q -e", sftpServer),
			}
			return cfg, nil
		},

		// CreateFn is a function that creates a temporary repository for the tests.
		Create: func(config interface{}) (restic.Backend, error) {
			cfg := config.(sftp.Config)
			return sftp.Create(cfg)
		},

		// OpenFn is a function that opens a previously created temporary repository.
		Open: func(config interface{}) (restic.Backend, error) {
			cfg := config.(sftp.Config)
			return sftp.Open(cfg)
		},

		// CleanupFn removes data created during the tests.
		Cleanup: func(config interface{}) error {
			cfg := config.(sftp.Config)
			if !TestCleanupTempDirs {
				t.Logf("leaving test backend dir at %v", cfg.Path)
			}

			RemoveAll(t, cfg.Path)
			return nil
		},
	}

	suite.RunTests(t)
}
