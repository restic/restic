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
	"github.com/restic/restic/internal/ui/progress"
)

func testRunInit(t testing.TB, gopts global.Options) {
	repository.TestUseLowSecurityKDFParameters(t)
	restic.TestDisableCheckPolynomial(t)
	restic.TestSetLockTimeout(t, 0)

	err := withTermStatus(t, gopts, func(ctx context.Context, gopts global.Options) error {
		return runInit(ctx, InitOptions{}, gopts, nil, gopts.Term)
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
		repo, err = global.OpenRepository(ctx, gopts, &progress.NoopPrinter{})
		return err
	})
	rtest.OK(t, err)

	var otherRepo *repository.Repository
	err = withTermStatus(t, env2.gopts, func(ctx context.Context, gopts global.Options) error {
		otherRepo, err = global.OpenRepository(ctx, gopts, &progress.NoopPrinter{})
		return err
	})
	rtest.OK(t, err)

	rtest.Assert(t, repo.Config().ChunkerPolynomial == otherRepo.Config().ChunkerPolynomial,
		"expected equal chunker polynomials, got %v expected %v", repo.Config().ChunkerPolynomial,
		otherRepo.Config().ChunkerPolynomial)
}
