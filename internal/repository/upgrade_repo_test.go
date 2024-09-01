package repository

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/errors"
	rtest "github.com/restic/restic/internal/test"
)

func TestUpgradeRepoV2(t *testing.T) {
	repo, _ := TestRepositoryWithVersion(t, 1)
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
