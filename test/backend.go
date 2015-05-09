package test_helper

import (
	"flag"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/backend/local"
	"github.com/restic/restic/repo"
)

var TestPassword = flag.String("test.password", "", `use this password for repositories created during tests (default: "geheim")`)
var TestCleanup = flag.Bool("test.cleanup", true, "clean up after running tests (remove local backend directory with all content)")
var TestTempDir = flag.String("test.tempdir", "", "use this directory for temporary storage (default: system temp dir)")

func SetupBackend(t testing.TB) *repo.Server {
	tempdir, err := ioutil.TempDir(*TestTempDir, "restic-test-")
	OK(t, err)

	// create repository below temp dir
	b, err := local.Create(filepath.Join(tempdir, "repo"))
	OK(t, err)

	// set cache dir below temp dir
	err = os.Setenv("RESTIC_CACHE", filepath.Join(tempdir, "cache"))
	OK(t, err)

	s := repo.NewServer(b)
	OK(t, s.Init(*TestPassword))
	return s
}

func TeardownBackend(t testing.TB, s *repo.Server) {
	if !*TestCleanup {
		l := s.Backend().(*local.Local)
		t.Logf("leaving local backend at %s\n", l.Location())
		return
	}

	OK(t, s.Delete())
}

func SnapshotDir(t testing.TB, server *repo.Server, path string, parent backend.ID) *restic.Snapshot {
	arch := restic.NewArchiver(server)
	sn, _, err := arch.Snapshot(nil, []string{path}, parent)
	OK(t, err)
	return sn
}
