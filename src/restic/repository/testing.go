package repository

import (
	"os"
	"restic/backend"
	"restic/backend/local"
	"restic/backend/mem"
	"restic/crypto"
	"testing"

	"github.com/restic/chunker"
)

// testKDFParams are the parameters for the KDF to be used during testing.
var testKDFParams = crypto.KDFParams{
	N: 128,
	R: 1,
	P: 1,
}

// TestUseLowSecurityKDFParameters configures low-security KDF parameters for testing.
func TestUseLowSecurityKDFParameters(t testing.TB) {
	t.Logf("using low-security KDF parameters for test")
	KDFParams = &testKDFParams
}

// TestBackend returns a fully configured in-memory backend.
func TestBackend(t testing.TB) (be backend.Backend, cleanup func()) {
	return mem.New(), func() {}
}

// TestPassword is used for all repositories created by the Test* functions.
const TestPassword = "geheim"

const testChunkerPol = chunker.Pol(0x3DA3358B4DC173)

// TestRepositoryWithBackend returns a repository initialized with a test
// password. If be is nil, an in-memory backend is used. A constant polynomial
// is used for the chunker and low-security test parameters.
func TestRepositoryWithBackend(t testing.TB, be backend.Backend) (r *Repository, cleanup func()) {
	TestUseLowSecurityKDFParameters(t)

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
