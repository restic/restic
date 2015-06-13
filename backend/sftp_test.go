package backend_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restic/restic/backend/sftp"
	. "github.com/restic/restic/test"
)

func setupSFTPBackend(t *testing.T) *sftp.SFTP {
	sftpserver := ""

	for _, dir := range strings.Split(TestSFTPPath, ":") {
		testpath := filepath.Join(dir, "sftp-server")
		fd, err := os.Open(testpath)
		fd.Close()
		if !os.IsNotExist(err) {
			sftpserver = testpath
			break
		}
	}

	if sftpserver == "" {
		return nil
	}

	tempdir, err := ioutil.TempDir("", "restic-test-")
	OK(t, err)

	b, err := sftp.Create(tempdir, sftpserver)
	OK(t, err)

	t.Logf("created sftp backend locally at %s", tempdir)

	return b
}

func teardownSFTPBackend(t *testing.T, b *sftp.SFTP) {
	if !TestCleanup {
		t.Logf("leaving backend at %s\n", b.Location())
		return
	}

	err := os.RemoveAll(b.Location())
	OK(t, err)
}

func TestSFTPBackend(t *testing.T) {
	if !RunIntegrationTest {
		t.Skip("integration tests disabled")
	}

	s := setupSFTPBackend(t)
	if s == nil {
		t.Skip("unable to find sftp-server binary")
		return
	}
	defer teardownSFTPBackend(t, s)

	testBackend(s, t)
}
