package repository

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/local"
	"github.com/restic/restic/internal/backend/mem"
	"github.com/restic/restic/internal/backend/retry"
	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"

	"github.com/restic/chunker"
)

type logger interface {
	Logf(format string, args ...interface{})
}

var paramsOnce sync.Once

// TestUseLowSecurityKDFParameters configures low-security KDF parameters for testing.
func TestUseLowSecurityKDFParameters(t logger) {
	t.Logf("using low-security KDF parameters for test")
	paramsOnce.Do(func() {
		params = &crypto.Params{
			N: 128,
			R: 1,
			P: 1,
		}
	})
}

// TestBackend returns a fully configured in-memory backend.
func TestBackend(_ testing.TB) backend.Backend {
	return mem.New()
}

const testChunkerPol = chunker.Pol(0x3DA3358B4DC173)

// TestRepositoryWithBackend returns a repository initialized with a test
// password. If be is nil, an in-memory backend is used. A constant polynomial
// is used for the chunker and low-security test parameters.
func TestRepositoryWithBackend(t testing.TB, be backend.Backend, version uint, opts Options) (*Repository, backend.Backend) {
	t.Helper()
	TestUseLowSecurityKDFParameters(t)
	restic.TestDisableCheckPolynomial(t)

	if be == nil {
		be = TestBackend(t)
	}

	repo, err := New(be, opts)
	if err != nil {
		t.Fatalf("TestRepository(): new repo failed: %v", err)
	}

	if version == 0 {
		version = restic.StableRepoVersion
	}
	pol := testChunkerPol
	err = repo.Init(context.TODO(), version, test.TestPassword, &pol)
	if err != nil {
		t.Fatalf("TestRepository(): initialize repo failed: %v", err)
	}

	return repo, be
}

// TestRepository returns a repository initialized with a test password on an
// in-memory backend. When the environment variable RESTIC_TEST_REPO is set to
// a non-existing directory, a local backend is created there and this is used
// instead. The directory is not removed, but left there for inspection.
func TestRepository(t testing.TB) *Repository {
	t.Helper()
	repo, _ := TestRepositoryWithVersion(t, 0)
	return repo
}

func TestRepositoryWithVersion(t testing.TB, version uint) (*Repository, backend.Backend) {
	t.Helper()
	dir := os.Getenv("RESTIC_TEST_REPO")
	opts := Options{}
	if dir != "" {
		_, err := os.Stat(dir)
		if err != nil {
			be, err := local.Create(context.TODO(), local.Config{Path: dir})
			if err != nil {
				t.Fatalf("error creating local backend at %v: %v", dir, err)
			}
			return TestRepositoryWithBackend(t, be, version, opts)
		}

		if err == nil {
			t.Logf("directory at %v already exists, using mem backend", dir)
		}
	}

	return TestRepositoryWithBackend(t, nil, version, opts)
}

func TestFromFixture(t testing.TB, repoFixture string) (*Repository, backend.Backend, func()) {
	repodir, cleanup := test.Env(t, repoFixture)
	repo, be := TestOpenLocal(t, repodir)

	return repo, be, cleanup
}

// TestOpenLocal opens a local repository.
func TestOpenLocal(t testing.TB, dir string) (*Repository, backend.Backend) {
	var be backend.Backend
	be, err := local.Open(context.TODO(), local.Config{Path: dir, Connections: 2})
	if err != nil {
		t.Fatal(err)
	}

	be = retry.New(be, 3, nil, nil)

	return TestOpenBackend(t, be), be
}

func TestOpenBackend(t testing.TB, be backend.Backend) *Repository {
	repo, err := New(be, Options{})
	if err != nil {
		t.Fatal(err)
	}
	err = repo.SearchKey(context.TODO(), test.TestPassword, 10, "")
	if err != nil {
		t.Fatal(err)
	}

	return repo
}

type VersionedTest func(t *testing.T, version uint)

func TestAllVersions(t *testing.T, test VersionedTest) {
	for version := restic.MinRepoVersion; version <= restic.MaxRepoVersion; version++ {
		t.Run(fmt.Sprintf("v%d", version), func(t *testing.T) {
			test(t, uint(version))
		})
	}
}

type VersionedBenchmark func(b *testing.B, version uint)

func BenchmarkAllVersions(b *testing.B, bench VersionedBenchmark) {
	for version := restic.MinRepoVersion; version <= restic.MaxRepoVersion; version++ {
		b.Run(fmt.Sprintf("v%d", version), func(b *testing.B) {
			bench(b, uint(version))
		})
	}
}
