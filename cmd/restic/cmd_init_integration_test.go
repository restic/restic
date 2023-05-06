package main

import (
	"context"
	"testing"

	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func testRunInit(t testing.TB, opts GlobalOptions) {
	repository.TestUseLowSecurityKDFParameters(t)
	restic.TestDisableCheckPolynomial(t)
	restic.TestSetLockTimeout(t, 0)

	rtest.OK(t, runInit(context.TODO(), InitOptions{}, opts, nil))
	t.Logf("repository initialized at %v", opts.Repo)
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
	rtest.Assert(t, runInit(context.TODO(), initOpts, env.gopts, nil) != nil, "expected invalid init options to fail")

	initOpts.CopyChunkerParameters = true
	rtest.OK(t, runInit(context.TODO(), initOpts, env.gopts, nil))

	repo, err := OpenRepository(context.TODO(), env.gopts)
	rtest.OK(t, err)

	otherRepo, err := OpenRepository(context.TODO(), env2.gopts)
	rtest.OK(t, err)

	rtest.Assert(t, repo.Config().ChunkerPolynomial == otherRepo.Config().ChunkerPolynomial,
		"expected equal chunker polynomials, got %v expected %v", repo.Config().ChunkerPolynomial,
		otherRepo.Config().ChunkerPolynomial)
}
