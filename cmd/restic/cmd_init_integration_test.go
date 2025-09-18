package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"
)

func testRunInit(t testing.TB, opts GlobalOptions) {
	repository.TestUseLowSecurityKDFParameters(t)
	restic.TestDisableCheckPolynomial(t)
	restic.TestSetLockTimeout(t, 0)

	err := withTermStatus(opts, func(ctx context.Context, term ui.Terminal) error {
		return runInit(ctx, InitOptions{}, opts, nil, term)
	})
	rtest.OK(t, err)
	t.Logf("repository initialized at %v", opts.Repo)

	// create temporary junk files to verify that restic does not trip over them
	for _, path := range []string{"index", "snapshots", "keys", "locks", filepath.Join("data", "00")} {
		rtest.OK(t, os.WriteFile(filepath.Join(opts.Repo, path, "tmp12345"), []byte("junk file"), 0o600))
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
	err := withTermStatus(env.gopts, func(ctx context.Context, term ui.Terminal) error {
		return runInit(ctx, initOpts, env.gopts, nil, term)
	})
	rtest.Assert(t, err != nil, "expected invalid init options to fail")

	initOpts.CopyChunkerParameters = true
	err = withTermStatus(env.gopts, func(ctx context.Context, term ui.Terminal) error {
		return runInit(ctx, initOpts, env.gopts, nil, term)
	})
	rtest.OK(t, err)

	repo, err := OpenRepository(context.TODO(), env.gopts, &progress.NoopPrinter{})
	rtest.OK(t, err)

	otherRepo, err := OpenRepository(context.TODO(), env2.gopts, &progress.NoopPrinter{})
	rtest.OK(t, err)

	rtest.Assert(t, repo.Config().ChunkerPolynomial == otherRepo.Config().ChunkerPolynomial,
		"expected equal chunker polynomials, got %v expected %v", repo.Config().ChunkerPolynomial,
		otherRepo.Config().ChunkerPolynomial)
}
