package sftp_test

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restic/restic/internal/backend"
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

func testConfig(path string) sftp.Config {
	return sftp.Config{
		Path:        path,
		Command:     fmt.Sprintf("%q -e", sftpServer),
		Connections: 5,
	}
}

func newTestSuite(t testing.TB) *test.Suite[sftp.Config] {
	return &test.Suite[sftp.Config]{
		// NewConfig returns a config for a new temporary backend that will be used in tests.
		NewConfig: func() (*sftp.Config, error) {
			dir := rtest.TempDir(t)
			t.Logf("create new backend at %v", dir)

			cfg := testConfig(dir)
			return &cfg, nil
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

func TestCreateSetsDirPermissions(t *testing.T) {
	if sftpServer == "" {
		t.Skip("sftp server binary not found")
	}

	cfg := testConfig(filepath.Join(rtest.TempDir(t), "repo"))

	be, err := sftp.Create(context.Background(), cfg, func(string, ...interface{}) {})
	rtest.OK(t, err)
	defer func() { rtest.OK(t, be.Close()) }()

	rtest.OK(t, filepath.WalkDir(cfg.Path, func(name string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		if mode := fi.Mode().Perm(); mode != 0o700 {
			return fmt.Errorf("directory %v has mode %o, want 700", name, mode)
		}
		return nil
	}))
}

func TestSaveSetsDirPermissions(t *testing.T) {
	if sftpServer == "" {
		t.Skip("sftp server binary not found")
	}

	cfg := testConfig(filepath.Join(rtest.TempDir(t), "repo"))

	be, err := sftp.Create(context.Background(), cfg, func(string, ...interface{}) {})
	rtest.OK(t, err)
	defer func() { rtest.OK(t, be.Close()) }()

	// Remove a data subdirectory so that Save has to recreate it.
	subdir := filepath.Join(cfg.Path, "data", "01")
	rtest.OK(t, os.Remove(subdir))

	data := []byte("test data")
	h := backend.Handle{Type: backend.PackFile, Name: "0100000000000000000000000000000000000000000000000000000000000000"}
	rtest.OK(t, be.Save(context.Background(), h, backend.NewByteReader(data, be.Hasher())))

	fi, err := os.Stat(subdir)
	rtest.OK(t, err)
	rtest.Equals(t, os.FileMode(0o700), fi.Mode().Perm())
}
