package repository

import (
	"os"
	"restic/backend"
	"restic/backend/local"
	"restic/backend/mem"
	"restic/patched/os"
	"testing"
)

// TestBackend returns a fully configured in-memory backend.
func TestBackend(t testing.TB) (be backend.Backend, cleanup func()) {
	return mem.New(), func() {}
}

// TestPassword is used for all repositories created by the Test* functions.
const TestPassword = "geheim"

// TestRepositoryWithBackend returns a repository initialized with a test
// password. If be is nil, an in-memory backend is used.
func TestRepositoryWithBackend(t testing.TB, be backend.Backend) (r *Repository, cleanup func()) {
	var beCleanup func()
	if be == nil {
		be, beCleanup = TestBackend(t)
	}

	r = New(be)

	err := r.Init(TestPassword)
	if err != nil {
		t.Fatalf("TestRepopository(): initialize repo failed: %v", err)
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
// instead. The directory is not removed.
func TestRepository(t testing.TB) (r *Repository, cleanup func()) {
	dir := os.Getenv("RESTIC_TEST_REPO")
	if dir != "" {
		_, err := patchedos.Stat(dir)
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
