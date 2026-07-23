package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func testRunInit(t testing.TB, gopts global.Options) {
	repository.TestUseLowSecurityKDFParameters(t)
	restic.TestDisableCheckPolynomial(t)
	repository.TestSetLockTimeout(t, 0)

	err := withTermStatus(t, gopts, func(ctx context.Context, gopts global.Options) error {
		return runInit(ctx, InitOptions{}, gopts, nil, gopts.Term)
	})
	rtest.OK(t, err)
	t.Logf("repository initialized at %v", gopts.Repo)

	// create temporary junk files to verify that restic does not trip over them
	for _, path := range []string{"index", "snapshots", "keys", "locks", filepath.Join("data", "00")} {
		rtest.OK(t, os.MkdirAll(filepath.Join(gopts.Repo, path), 0700))
		rtest.OK(t, os.WriteFile(filepath.Join(gopts.Repo, path, "tmp12345"), []byte("junk file"), 0o600))
	}
}

func TestInitCopyChunkerParams(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	env2, cleanup2 := withTestEnvironment(t)
	defer cleanup2()

	testRunInit(t, env2.gopts)

	initOpts := InitOptions{
		SecondaryRepoOptions: global.SecondaryRepoOptions{
			Repo:     env2.gopts.Repo,
			Password: env2.gopts.Password,
		},
	}
	err := withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		return runInit(ctx, initOpts, gopts, nil, gopts.Term)
	})
	rtest.Assert(t, err != nil, "expected invalid init options to fail")

	initOpts.CopyChunkerParameters = true
	err = withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		return runInit(ctx, initOpts, gopts, nil, gopts.Term)
	})
	rtest.OK(t, err)

	var repo *repository.Repository
	err = withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		repo, err = global.OpenRepository(ctx, gopts, restic.NewNoopPrinter())
		return err
	})
	rtest.OK(t, err)

	var otherRepo *repository.Repository
	err = withTermStatus(t, env2.gopts, func(ctx context.Context, gopts global.Options) error {
		otherRepo, err = global.OpenRepository(ctx, gopts, restic.NewNoopPrinter())
		return err
	})
	rtest.OK(t, err)

	rtest.Assert(t, repo.Config().ChunkerPolynomial == otherRepo.Config().ChunkerPolynomial,
		"expected equal chunker polynomials, got %v expected %v", repo.Config().ChunkerPolynomial,
		otherRepo.Config().ChunkerPolynomial)
}

// TestOpenRepositoryCacheUnavailable verifies that OpenRepository continues
// without a cache, rather than aborting, when the cache cannot be opened. This
// happens for example when restic runs as a systemd system service without
// $HOME set, or when the cache directory is not writable. Opening the cache is
// best-effort: restic only prints a warning and proceeds.
func TestOpenRepositoryCacheUnavailable(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testRunInit(t, env.gopts)

	// Point the cache directory below a regular file so that creating it fails.
	// cache.New then returns an error at the same spot it does when no cache
	// directory can be located at all (missing $HOME / $XDG_CACHE_HOME).
	blocker := filepath.Join(env.base, "not-a-directory")
	rtest.OK(t, os.WriteFile(blocker, []byte("x"), 0o600))
	env.gopts.CacheDir = filepath.Join(blocker, "cache")

	var repo *repository.Repository
	err := withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		var err error
		repo, err = global.OpenRepository(ctx, gopts, restic.NewNoopPrinter())
		return err
	})
	rtest.OK(t, err)
	rtest.Assert(t, repo != nil, "expected a usable repository even when the cache is unavailable")
}
