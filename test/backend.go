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
	"github.com/restic/restic/repository"
)

var TestPassword = flag.String("test.password", "geheim", `use this password for repositories created during tests (default: "geheim")`)
var TestCleanup = flag.Bool("test.cleanup", true, "clean up after running tests (remove local backend directory with all content)")
var TestTempDir = flag.String("test.tempdir", "", "use this directory for temporary storage (default: system temp dir)")

func SetupRepo(t testing.TB) *repository.Repo {
	tempdir, err := ioutil.TempDir(*TestTempDir, "restic-test-")
	OK(t, err)

	// create repository below temp dir
	b, err := local.Create(filepath.Join(tempdir, "repo"))
	OK(t, err)

	// set cache dir below temp dir
	err = os.Setenv("RESTIC_CACHE", filepath.Join(tempdir, "cache"))
	OK(t, err)

	repo := repository.New(b)
	OK(t, repo.Init(*TestPassword))
	return repo
}

func TeardownRepo(t testing.TB, repo *repository.Repo) {
	if !*TestCleanup {
		l := repo.Backend().(*local.Local)
		t.Logf("leaving local backend at %s\n", l.Location())
		return
	}

	OK(t, repo.Delete())
}

func SnapshotDir(t testing.TB, repo *repository.Repo, path string, parent backend.ID) *restic.Snapshot {
	arch := restic.NewArchiver(repo)
	sn, _, err := arch.Snapshot(nil, []string{path}, parent)
	OK(t, err)
	return sn
}
