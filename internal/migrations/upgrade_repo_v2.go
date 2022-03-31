package migrations

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

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

func (*UpgradeRepoV2) upgrade(ctx context.Context, repo restic.Repository) error {
	h := restic.Handle{Type: restic.ConfigFile}

	// now remove the original file
	err := repo.Backend().Remove(ctx, h)
	if err != nil {
		return fmt.Errorf("remove config failed: %w", err)
	}

	// upgrade config
	cfg := repo.Config()
	cfg.Version = 2

	_, err = repo.SaveJSONUnpacked(ctx, restic.ConfigFile, cfg)
	if err != nil {
		return fmt.Errorf("save new config file failed: %w", err)
	}

	return nil
}

func (m *UpgradeRepoV2) Apply(ctx context.Context, repo restic.Repository) error {
	tempdir, err := ioutil.TempDir("", "restic-migrate-upgrade-repo-v2-")
	if err != nil {
		return fmt.Errorf("create temp dir failed: %w", err)
	}

	h := restic.Handle{Type: restic.ConfigFile}

	// read raw config file and save it to a temp dir, just in case
	var rawConfigFile []byte
	err = repo.Backend().Load(ctx, h, 0, 0, func(rd io.Reader) (err error) {
		rawConfigFile, err = ioutil.ReadAll(rd)
		return err
	})
	if err != nil {
		return fmt.Errorf("load config file failed: %w", err)
	}

	err = ioutil.WriteFile(filepath.Join(tempdir, "config.old"), rawConfigFile, 0600)
	if err != nil {
		return fmt.Errorf("write config file backup to %v failed: %w", tempdir, err)
	}

	// run the upgrade
	err = m.upgrade(ctx, repo)
	if err != nil {
		// try contingency methods, reupload the original file
		_ = repo.Backend().Remove(ctx, h)
		uploadError := repo.Backend().Save(ctx, h, restic.NewByteReader(rawConfigFile, nil))
		if uploadError != nil {
			return fmt.Errorf("error uploading config (%w), re-uploading old config filed failed as well (%v) but there is a backup in %v", err, uploadError, tempdir)
		}

		return fmt.Errorf("error uploading config (%w), re-uploadid old config, there is a backup in %v", err, tempdir)
	}

	_ = os.Remove(backupFileName)
	_ = os.Remove(tempdir)
	return nil
}
