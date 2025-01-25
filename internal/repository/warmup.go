package repository

import (
	"context"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/restic"
)

// NewWarmupJob creates a new warmup job, requesting the backend to warmup the specified packs.
func (repo *Repository) NewWarmupJob(ctx context.Context, packs restic.IDSet) (restic.WarmupJob, error) {
	handles := make([]backend.Handle, 0, len(packs))
	for pack := range packs {
		handles = append(
			handles,
			backend.Handle{Type: restic.PackFile, Name: pack.String()},
		)
	}
	handlesWarmingUp, err := repo.be.Warmup(ctx, handles)
	return restic.WarmupJob{HandlesWarmingUp: handlesWarmingUp}, err
}

// WaitWarmupJob waits for the warmup job to complete.
func (repo *Repository) WaitWarmupJob(ctx context.Context, job restic.WarmupJob) error {
	return repo.be.WarmupWait(ctx, job.HandlesWarmingUp)
}
