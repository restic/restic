package repository

import (
	"context"
	"os"
	"testing"

	"github.com/restic/restic/internal/backend/local"
	"github.com/restic/restic/internal/backend/mem"
	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"

	"github.com/restic/chunker"
)

// testKDFParams are the parameters for the KDF to be used during testing.
var testKDFParams = crypto.Params{
	N: 128,
	R: 1,
	P: 1,
}

type logger interface {
	Logf(format string, args ...interface{})
}

// TestUseLowSecurityKDFParameters configures low-security KDF parameters for testing.
func TestUseLowSecurityKDFParameters(t logger) {
	t.Logf("using low-security KDF parameters for test")
	Params = &testKDFParams
}

// TestBackend returns a fully configured in-memory backend.
func TestBackend(t testing.TB) (be restic.Backend, cleanup func()) {
	return mem.New(), func() {}
}

const testChunkerPol = chunker.Pol(0x3DA3358B4DC173)

// TestRepositoryWithBackend returns a repository initialized with a test
// password. If be is nil, an in-memory backend is used. A constant polynomial
// is used for the chunker and low-security test parameters.
func TestRepositoryWithBackend(t testing.TB, be restic.Backend) (r restic.Repository, cleanup func()) {
	test.Helper(t).Helper()
	TestUseLowSecurityKDFParameters(t)
	restic.TestDisableCheckPolynomial(t)

	var beCleanup func()
	if be == nil {
		be, beCleanup = TestBackend(t)
	}

	repo := New(be)

	cfg := restic.TestCreateConfig(t, testChunkerPol)
	err := repo.init(context.TODO(), test.TestPassword, cfg)
	if err != nil {
		t.Fatalf("TestRepository(): initialize repo failed: %v", err)
	}

	return repo, func() {
		if beCleanup != nil {
			beCleanup()
		}
	}
}

// TestRepository returns a repository initialized with a test password on an
// in-memory backend. When the environment variable RESTIC_TEST_REPO is set to
// a non-existing directory, a local backend is created there and this is used
// instead. The directory is not removed, but left there for inspection.
func TestRepository(t testing.TB) (r restic.Repository, cleanup func()) {
	test.Helper(t).Helper()
	dir := os.Getenv("RESTIC_TEST_REPO")
	if dir != "" {
		_, err := os.Stat(dir)
		if err != nil {
			be, err := local.Create(local.Config{Path: dir})
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

// TestOpenLocal opens a local repository.
func TestOpenLocal(t testing.TB, dir string) (r restic.Repository) {
	be, err := local.Open(local.Config{Path: dir})
	if err != nil {
		t.Fatal(err)
	}

	repo := New(be)
	err = repo.SearchKey(context.TODO(), test.TestPassword, 10, "")
	if err != nil {
		t.Fatal(err)
	}

	return repo
}
