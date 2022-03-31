package migrations

import (
	"context"
	"fmt"

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

func (*UpgradeRepoV2) Check(ctx context.Context, repo restic.Repository) (bool, error) {
	isV1 := repo.Config().Version == 1
	return isV1, nil
}

func (*UpgradeRepoV2) Apply(ctx context.Context, repo restic.Repository) error {
	cfg := repo.Config()
	cfg.Version = 2

	h := restic.Handle{Type: restic.ConfigFile}

	err := repo.Backend().Remove(ctx, h)
	if err != nil {
		return fmt.Errorf("remove old config file failed: %w", err)
	}

	_, err = repo.SaveJSONUnpacked(ctx, restic.ConfigFile, cfg)
	if err != nil {
		return fmt.Errorf("save new config file failed: %w", err)
	}

	return nil
}
