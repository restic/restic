package test_helper

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"restic"
	"restic/backend"
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

func SetupRepo() *repository.Repository {
	tempdir, err := ioutil.TempDir(TestTempDir, "restic-test-")
	if err != nil {
		panic(err)
	}

	// create repository below temp dir
	b, err := local.Create(filepath.Join(tempdir, "repo"))
	if err != nil {
		panic(err)
	}

	repo, err := repository.New(b)
	if err != nil {
		panic(err)
	}
	err = repo.Init(TestPassword)
	if err != nil {
		panic(err)
	}

	return repo
}

func TeardownRepo(repo *repository.Repository) {
	if !TestCleanupTempDirs {
		l := repo.Backend().(*local.Local)
		fmt.Printf("leaving local backend at %s\n", l.Location())
		return
	}

	err := repo.Delete()
	if err != nil {
		panic(err)
	}
}

func SnapshotDir(t testing.TB, repo *repository.Repository, path string, parent *backend.ID) *restic.Snapshot {
	arch := restic.NewArchiver(repo)
	sn, _, err := arch.Snapshot(nil, []string{path}, parent)
	OK(t, err)
	return sn
}

func WithRepo(t testing.TB, f func(*repository.Repository)) {
	repo := SetupRepo()
	f(repo)
	TeardownRepo(repo)
}
