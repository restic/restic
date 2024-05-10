package repository

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/restic"
)

type upgradeRepoV2Error struct {
	UploadNewConfigError   error
	ReuploadOldConfigError error

	BackupFilePath string
}

func (err *upgradeRepoV2Error) Error() string {
	if err.ReuploadOldConfigError != nil {
		return fmt.Sprintf("error uploading config (%v), re-uploading old config filed failed as well (%v), but there is a backup of the config file in %v", err.UploadNewConfigError, err.ReuploadOldConfigError, err.BackupFilePath)
	}

	return fmt.Sprintf("error uploading config (%v), re-uploaded old config was successful, there is a backup of the config file in %v", err.UploadNewConfigError, err.BackupFilePath)
}

func (err *upgradeRepoV2Error) Unwrap() error {
	// consider the original upload error as the primary cause
	return err.UploadNewConfigError
}

func upgradeRepository(ctx context.Context, repo *Repository) error {
	h := backend.Handle{Type: backend.ConfigFile}

	if !repo.be.HasAtomicReplace() {
		// remove the original file for backends which do not support atomic overwriting
		err := repo.be.Remove(ctx, h)
		if err != nil {
			return fmt.Errorf("remove config failed: %w", err)
		}
	}

	// upgrade config
	cfg := repo.Config()
	cfg.Version = 2

	err := restic.SaveConfig(ctx, repo, cfg)
	if err != nil {
		return fmt.Errorf("save new config file failed: %w", err)
	}

	return nil
}

func UpgradeRepo(ctx context.Context, repo *Repository) error {
	if repo.Config().Version != 1 {
		return fmt.Errorf("repository has version %v, only upgrades from version 1 are supported", repo.Config().Version)
	}

	tempdir, err := os.MkdirTemp("", "restic-migrate-upgrade-repo-v2-")
	if err != nil {
		return fmt.Errorf("create temp dir failed: %w", err)
	}

	h := backend.Handle{Type: restic.ConfigFile}

	// read raw config file and save it to a temp dir, just in case
	rawConfigFile, err := repo.LoadRaw(ctx, restic.ConfigFile, restic.ID{})
	if err != nil {
		return fmt.Errorf("load config file failed: %w", err)
	}

	backupFileName := filepath.Join(tempdir, "config")
	err = os.WriteFile(backupFileName, rawConfigFile, 0600)
	if err != nil {
		return fmt.Errorf("write config file backup to %v failed: %w", tempdir, err)
	}

	// run the upgrade
	err = upgradeRepository(ctx, repo)
	if err != nil {

		// build an error we can return to the caller
		repoError := &upgradeRepoV2Error{
			UploadNewConfigError: err,
			BackupFilePath:       backupFileName,
		}

		// try contingency methods, reupload the original file
		_ = repo.be.Remove(ctx, h)
		err = repo.be.Save(ctx, h, backend.NewByteReader(rawConfigFile, nil))
		if err != nil {
			repoError.ReuploadOldConfigError = err
		}

		return repoError
	}

	_ = os.Remove(backupFileName)
	_ = os.Remove(tempdir)
	return nil
}
