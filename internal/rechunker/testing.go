package rechunker

import (
	"context"
	"testing"

	"github.com/restic/chunker"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
)

func TestRepositoryWithPol(t *testing.T, pol chunker.Pol) restic.Repository {
	t.Helper()

	be := repository.TestBackend(t)

	repo, err := repository.New(be, repository.Options{})
	if err != nil {
		t.Fatalf("TestRepository(): new repo failed: %v", err)
	}

	var version uint = restic.StableRepoVersion
	err = repo.Init(context.TODO(), version, test.TestPassword, &pol)
	if err != nil {
		t.Fatalf("TestRepository(): initialize repo failed: %v", err)
	}

	return repo
}
