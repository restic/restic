package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui/progress"
)

func testRunInit(t testing.TB, gopts GlobalOptions) {
	repository.TestUseLowSecurityKDFParameters(t)
	restic.TestDisableCheckPolynomial(t)
	restic.TestSetLockTimeout(t, 0)

	err := withTermStatus(t, gopts, func(ctx context.Context, gopts GlobalOptions) error {
		return runInit(ctx, InitOptions{}, gopts, nil, gopts.term)
	})
	rtest.OK(t, err)
	t.Logf("repository initialized at %v", gopts.Repo)

	// create temporary junk files to verify that restic does not trip over them
	for _, path := range []string{"index", "snapshots", "keys", "locks", filepath.Join("data", "00")} {
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
		secondaryRepoOptions: secondaryRepoOptions{
			Repo:     env2.gopts.Repo,
			password: env2.gopts.password,
		},
	}
	err := withTermStatus(t, env.gopts, func(ctx context.Context, gopts GlobalOptions) error {
		return runInit(ctx, initOpts, gopts, nil, gopts.term)
	})
	rtest.Assert(t, err != nil, "expected invalid init options to fail")

	initOpts.CopyChunkerParameters = true
	err = withTermStatus(t, env.gopts, func(ctx context.Context, gopts GlobalOptions) error {
		return runInit(ctx, initOpts, gopts, nil, gopts.term)
	})
	rtest.OK(t, err)

	var repo *repository.Repository
	err = withTermStatus(t, env.gopts, func(ctx context.Context, gopts GlobalOptions) error {
		repo, err = OpenRepository(ctx, gopts, &progress.NoopPrinter{})
		return err
	})
	rtest.OK(t, err)

	var otherRepo *repository.Repository
	err = withTermStatus(t, env2.gopts, func(ctx context.Context, gopts GlobalOptions) error {
		otherRepo, err = OpenRepository(ctx, gopts, &progress.NoopPrinter{})
		return err
	})
	rtest.OK(t, err)

	rtest.Assert(t, repo.Config().ChunkerPolynomial == otherRepo.Config().ChunkerPolynomial,
		"expected equal chunker polynomials, got %v expected %v", repo.Config().ChunkerPolynomial,
		otherRepo.Config().ChunkerPolynomial)
}
