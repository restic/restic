package repository

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/local"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func TestUpgradeRepoV2(t *testing.T) {
	repo, _, _ := TestRepositoryWithVersion(t, 1)
	if repo.Config().Version != 1 {
		t.Fatal("test repo has wrong version")
	}

	err := UpgradeRepo(context.Background(), repo)
	rtest.OK(t, err)
}

type failBackend struct {
	backend.Backend

	mu                        sync.Mutex
	ConfigFileSavesUntilError uint
}

func (be *failBackend) Save(ctx context.Context, h backend.Handle, rd backend.RewindReader) error {
	if h.Type != backend.ConfigFile {
		return be.Backend.Save(ctx, h, rd)
	}

	be.mu.Lock()
	if be.ConfigFileSavesUntilError == 0 {
		be.mu.Unlock()
		return errors.New("failure induced for testing")
	}

	be.ConfigFileSavesUntilError--
	be.mu.Unlock()

	return be.Backend.Save(ctx, h, rd)
}

func TestUpgradeRepoV2FailurePreservesLocalConfig(t *testing.T) {
	ctx := context.Background()
	be, err := local.Create(ctx, local.Config{Path: t.TempDir()}, t.Logf)
	rtest.OK(t, err)

	// Both the upgrade and the recovery write fail after initialization.
	repo, _ := TestRepositoryWithBackend(t, &failBackend{
		ConfigFileSavesUntilError: 1,
		Backend:                   be,
	}, 1, Options{})
	originalConfig, err := repo.LoadRaw(ctx, restic.ConfigFile, restic.ID{})
	rtest.OK(t, err)

	err = UpgradeRepo(ctx, repo)
	if err == nil {
		t.Fatal("expected upgrade to fail")
	}
	upgradeErr := err.(*upgradeRepoV2Error)
	t.Cleanup(func() {
		rtest.OK(t, os.Remove(upgradeErr.BackupFilePath))
		rtest.OK(t, os.Remove(filepath.Dir(upgradeErr.BackupFilePath)))
	})
	if upgradeErr.ReuploadOldConfigError == nil {
		t.Fatal("expected recovery write to fail")
	}

	// A failed recovery must not delete the intact original config.
	preservedConfig, err := repo.LoadRaw(ctx, restic.ConfigFile, restic.ID{})
	rtest.OK(t, err)
	rtest.Equals(t, originalConfig, preservedConfig)
	reopened, _ := TestOpenLocal(t, be.Path)
	rtest.Equals(t, repo.Config(), reopened.Config())
}

func TestUpgradeRepoV2Failure(t *testing.T) {
	be := TestBackend(t)

	// wrap backend so that it fails upgrading the config after the initial write
	be = &failBackend{
		ConfigFileSavesUntilError: 1,
		Backend:                   be,
	}

	repo, _ := TestRepositoryWithBackend(t, be, 1, Options{})
	if repo.Config().Version != 1 {
		t.Fatal("test repo has wrong version")
	}

	err := UpgradeRepo(context.Background(), repo)
	if err == nil {
		t.Fatal("expected error returned from Apply(), got nil")
	}

	upgradeErr := err.(*upgradeRepoV2Error)
	if upgradeErr.UploadNewConfigError == nil {
		t.Fatal("expected upload error, got nil")
	}

	if upgradeErr.ReuploadOldConfigError == nil {
		t.Fatal("expected reupload error, got nil")
	}

	if upgradeErr.BackupFilePath == "" {
		t.Fatal("no backup file path found")
	}
	rtest.OK(t, os.Remove(upgradeErr.BackupFilePath))
	rtest.OK(t, os.Remove(filepath.Dir(upgradeErr.BackupFilePath)))
}
