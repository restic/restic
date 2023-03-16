package migrations

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
)

func TestUpgradeRepoV2(t *testing.T) {
	repo := repository.TestRepositoryWithVersion(t, 1)
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

type failBackend struct {
	restic.Backend

	mu                        sync.Mutex
	ConfigFileSavesUntilError uint
}

func (be *failBackend) Save(ctx context.Context, h restic.Handle, rd restic.RewindReader) error {
	if h.Type != restic.ConfigFile {
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
	be := repository.TestBackend(t)

	// wrap backend so that it fails upgrading the config after the initial write
	be = &failBackend{
		ConfigFileSavesUntilError: 1,
		Backend:                   be,
	}

	repo := repository.TestRepositoryWithBackend(t, be, 1)
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
	if err == nil {
		t.Fatal("expected error returned from Apply(), got nil")
	}

	upgradeErr := err.(*UpgradeRepoV2Error)
	if upgradeErr.UploadNewConfigError == nil {
		t.Fatal("expected upload error, got nil")
	}

	if upgradeErr.ReuploadOldConfigError == nil {
		t.Fatal("expected reupload error, got nil")
	}

	if upgradeErr.BackupFilePath == "" {
		t.Fatal("no backup file path found")
	}
	test.OK(t, os.Remove(upgradeErr.BackupFilePath))
	test.OK(t, os.Remove(filepath.Dir(upgradeErr.BackupFilePath)))
}
