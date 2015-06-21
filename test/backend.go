package test_helper

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/backend/local"
	"github.com/restic/restic/repository"
)

var (
	TestPassword       = getStringVar("RESTIC_TEST_PASSWORD", "geheim")
	TestCleanup        = getBoolVar("RESTIC_TEST_CLEANUP", true)
	TestTempDir        = getStringVar("RESTIC_TEST_TMPDIR", "")
	RunIntegrationTest = getBoolVar("RESTIC_TEST_INTEGRATION", true)
	TestSFTPPath       = getStringVar("RESTIC_TEST_SFTPPATH",
		"/usr/lib/ssh:/usr/lib/openssh")
)

func getStringVar(name, defaultValue string) string {
	if e := os.Getenv(name); e != "" {
		return e
	}

	return defaultValue
}

func getBoolVar(name string, defaultValue bool) bool {
	if e := os.Getenv(name); e != "" {
		switch e {
		case "1":
			return true
		case "0":
			return false
		default:
			fmt.Fprintf(os.Stderr, "invalid value for variable %q, using default\n", name)
		}
	}

	return defaultValue
}

func SetupRepo(t testing.TB) *repository.Repository {
	tempdir, err := ioutil.TempDir(TestTempDir, "restic-test-")
	OK(t, err)

	// create repository below temp dir
	b, err := local.Create(filepath.Join(tempdir, "repo"))
	OK(t, err)

	repo := repository.New(b)
	OK(t, repo.Init(TestPassword))
	return repo
}

func TeardownRepo(t testing.TB, repo *repository.Repository) {
	if !TestCleanup {
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
