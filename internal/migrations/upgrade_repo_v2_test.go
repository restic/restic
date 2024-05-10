package migrations

import (
	"context"
	"testing"

	"github.com/restic/restic/internal/repository"
)

func TestUpgradeRepoV2(t *testing.T) {
	repo, _ := repository.TestRepositoryWithVersion(t, 1)
	if repo.Config().Version != 1 {
		t.Fatal("test repo has wrong version")
	}

	m := &UpgradeRepoV2{}

	ok, _, err := m.Check(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}

	if !ok {
		t.Fatal("migration check returned false")
	}

	err = m.Apply(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
}
