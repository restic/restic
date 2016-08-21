package repository

import (
	"os"
	"restic/backend"
	"restic/backend/local"
	"restic/backend/mem"
	"testing"

	"github.com/restic/chunker"
)

// TestBackend returns a fully configured in-memory backend.
func TestBackend(t testing.TB) (be backend.Backend, cleanup func()) {
	return mem.New(), func() {}
}

// TestPassword is used for all repositories created by the Test* functions.
const TestPassword = "geheim"

const testChunkerPol = chunker.Pol(0x3DA3358B4DC173)

// TestRepositoryWithBackend returns a repository initialized with a test
// password. If be is nil, an in-memory backend is used. A constant polynomial
// is used for the chunker.
func TestRepositoryWithBackend(t testing.TB, be backend.Backend) (r *Repository, cleanup func()) {
	var beCleanup func()
	if be == nil {
		be, beCleanup = TestBackend(t)
	}

	r = New(be)

	cfg := TestCreateConfig(t, testChunkerPol)
	err := r.init(TestPassword, cfg)
	if err != nil {
		t.Fatalf("TestRepository(): initialize repo failed: %v", err)
	}

	return r, func() {
		if beCleanup != nil {
			beCleanup()
		}
	}
}

// TestRepository returns a repository initialized with a test password on an
// in-memory backend. When the environment variable RESTIC_TEST_REPO is set to
// a non-existing directory, a local backend is created there and this is used
// instead. The directory is not removed, but left there for inspection.
func TestRepository(t testing.TB) (r *Repository, cleanup func()) {
	dir := os.Getenv("RESTIC_TEST_REPO")
	if dir != "" {
		_, err := os.Stat(dir)
		if err != nil {
			be, err := local.Create(dir)
			if err != nil {
				t.Fatalf("error creating local backend at %v: %v", dir, err)
			}
			return TestRepositoryWithBackend(t, be)
		}

		if err == nil {
			t.Logf("directory at %v already exists, using mem backend", dir)
		}
	}

	return TestRepositoryWithBackend(t, nil)
}
