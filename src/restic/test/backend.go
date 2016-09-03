package test_helper

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"restic"
	"restic/archiver"
	"restic/backend/local"
	"restic/repository"
)

var (
	TestPassword          = getStringVar("RESTIC_TEST_PASSWORD", "geheim")
	TestCleanupTempDirs   = getBoolVar("RESTIC_TEST_CLEANUP", true)
	TestTempDir           = getStringVar("RESTIC_TEST_TMPDIR", "")
	RunIntegrationTest    = getBoolVar("RESTIC_TEST_INTEGRATION", true)
	RunFuseTest           = getBoolVar("RESTIC_TEST_FUSE", true)
	TestSFTPPath          = getStringVar("RESTIC_TEST_SFTPPATH", "/usr/lib/ssh:/usr/lib/openssh")
	TestWalkerPath        = getStringVar("RESTIC_TEST_PATH", ".")
	BenchArchiveDirectory = getStringVar("RESTIC_BENCH_DIR", ".")
	TestS3Server          = getStringVar("RESTIC_TEST_S3_SERVER", "")
	TestRESTServer        = getStringVar("RESTIC_TEST_REST_SERVER", "")
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

// SetupRepo returns a repo setup in a temp dir.
func SetupRepo() restic.Repository {
	tempdir, err := ioutil.TempDir(TestTempDir, "restic-test-")
	if err != nil {
		panic(err)
	}

	// create repository below temp dir
	b, err := local.Create(filepath.Join(tempdir, "repo"))
	if err != nil {
		panic(err)
	}

	repo := repository.New(b)
	err = repo.Init(TestPassword)
	if err != nil {
		panic(err)
	}

	return repo
}

// TeardownRepo removes a repository created by SetupRepo.
func TeardownRepo(repo restic.Repository) {
	if !TestCleanupTempDirs {
		l := repo.Backend().(*local.Local)
		fmt.Printf("leaving local backend at %s\n", l.Location())
		return
	}

	if r, ok := repo.(restic.Deleter); ok {
		err := r.Delete()
		if err != nil {
			panic(err)
		}
	}
}

// SnapshotDir creates a new snapshot of path.
func SnapshotDir(t testing.TB, repo restic.Repository, path string, parent *restic.ID) *restic.Snapshot {
	arch := archiver.New(repo)
	sn, _, err := arch.Snapshot(nil, []string{path}, parent)
	OK(t, err)
	return sn
}

// WithRepo runs the function t with a repository that is removed after f returns.
func WithRepo(t testing.TB, f func(restic.Repository)) {
	repo := SetupRepo()
	f(repo)
	TeardownRepo(repo)
}
