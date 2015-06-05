package test_helper

import (
	"flag"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/backend/local"
	"github.com/restic/restic/repository"
)

var (
	TestPassword       = flag.String("test.password", "geheim", `use this password for repositories created during tests (default: "geheim")`)
	TestCleanup        = flag.Bool("test.cleanup", true, "clean up after running tests (remove local backend directory with all content)")
	TestTempDir        = flag.String("test.tempdir", "", "use this directory for temporary storage (default: system temp dir)")
	RunIntegrationTest = flag.Bool("test.integration", false, "run integration tests (default: false)")
)

func SetupRepo(t testing.TB) *repository.Repository {
	tempdir, err := ioutil.TempDir(*TestTempDir, "restic-test-")
	OK(t, err)

	// create repository below temp dir
	b, err := local.Create(filepath.Join(tempdir, "repo"))
	OK(t, err)

	repo := repository.New(b)
	OK(t, repo.Init(*TestPassword))
	return repo
}

func TeardownRepo(t testing.TB, repo *repository.Repository) {
	if !*TestCleanup {
		l := repo.Backend().(*local.Local)
		t.Logf("leaving local backend at %s\n", l.Location())
		return
	}

	OK(t, repo.Delete())
}

func SnapshotDir(t testing.TB, repo *repository.Repository, path string, parent backend.ID) *restic.Snapshot {
	arch := restic.NewArchiver(repo)
	sn, _, err := arch.Snapshot(nil, []string{path}, parent)
	OK(t, err)
	return sn
}
