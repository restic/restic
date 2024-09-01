package migrations

import (
	"context"
	"fmt"

	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
)

func init() {
	register(&UpgradeRepoV2{})
}

type UpgradeRepoV2 struct{}

func (*UpgradeRepoV2) Name() string {
	return "upgrade_repo_v2"
}

func (*UpgradeRepoV2) Desc() string {
	return "upgrade a repository to version 2"
}

func (*UpgradeRepoV2) Check(_ context.Context, repo restic.Repository) (bool, string, error) {
	isV1 := repo.Config().Version == 1
	reason := ""
	if !isV1 {
		reason = fmt.Sprintf("repository is already upgraded to version %v", repo.Config().Version)
	}
	return isV1, reason, nil
}

func (*UpgradeRepoV2) RepoCheck() bool {
	return true
}

func (m *UpgradeRepoV2) Apply(ctx context.Context, repo restic.Repository) error {
	return repository.UpgradeRepo(ctx, repo.(*repository.Repository))
}
