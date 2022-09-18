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

type UpgradeRepoV2Error struct {
	UploadNewConfigError   error
	ReuploadOldConfigError error

	BackupFilePath string
}

func (err *UpgradeRepoV2Error) Error() string {
	if err.ReuploadOldConfigError != nil {
		return fmt.Sprintf("error uploading config (%v), re-uploading old config filed failed as well (%v), but there is a backup of the config file in %v", err.UploadNewConfigError, err.ReuploadOldConfigError, err.BackupFilePath)
	}

	return fmt.Sprintf("error uploading config (%v), re-uploaded old config was successful, there is a backup of the config file in %v", err.UploadNewConfigError, err.BackupFilePath)
}

func (err *UpgradeRepoV2Error) Unwrap() error {
	// consider the original upload error as the primary cause
	return err.UploadNewConfigError
}

type UpgradeRepoV2 struct{}

func (*UpgradeRepoV2) Name() string {
	return "upgrade_repo_v2"
}

func (*UpgradeRepoV2) Desc() string {
	return "upgrade a repository to version 2"
}

func (*UpgradeRepoV2) Check(ctx context.Context, repo restic.Repository) (bool, string, error) {
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
func (*UpgradeRepoV2) upgrade(ctx context.Context, repo restic.Repository) error {
	h := restic.Handle{Type: restic.ConfigFile}

	if !repo.Backend().HasAtomicReplace() {
		// remove the original file for backends which do not support atomic overwriting
		err := repo.Backend().Remove(ctx, h)
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

	backupFileName := filepath.Join(tempdir, "config")
	err = ioutil.WriteFile(backupFileName, rawConfigFile, 0600)
	if err != nil {
		return fmt.Errorf("write config file backup to %v failed: %w", tempdir, err)
	}

	// run the upgrade
	err = m.upgrade(ctx, repo)
	if err != nil {

		// build an error we can return to the caller
		repoError := &UpgradeRepoV2Error{
			UploadNewConfigError: err,
			BackupFilePath:       backupFileName,
		}

		// try contingency methods, reupload the original file
		_ = repo.Backend().Remove(ctx, h)
		err = repo.Backend().Save(ctx, h, restic.NewByteReader(rawConfigFile, nil))
		if err != nil {
			repoError.ReuploadOldConfigError = err
		}

		return repoError
	}

	_ = os.Remove(backupFileName)
	_ = os.Remove(tempdir)
	return nil
}
