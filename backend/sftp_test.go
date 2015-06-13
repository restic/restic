package backend_test

import (
	"flag"
	"io/ioutil"
	"os"
	"testing"

	"github.com/restic/restic/backend/sftp"
	. "github.com/restic/restic/test"
)

var sftpPath = flag.String("test.sftppath", "", "sftp binary path (default: empty)")

func setupSFTPBackend(t *testing.T) *sftp.SFTP {
	tempdir, err := ioutil.TempDir("", "restic-test-")
	OK(t, err)

	b, err := sftp.Create(tempdir, *sftpPath)
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
		t.Skip("integration tests disabled, use `-test.integration` to enable")
	}

	if *sftpPath == "" {
		t.Skipf("sftppath not set, skipping TestSFTPBackend")
	}

	s := setupSFTPBackend(t)
	defer teardownSFTPBackend(t, s)

	testBackend(s, t)
}
