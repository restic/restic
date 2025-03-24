package repository

import (
	"context"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/restic"
)

type WarmupJob struct {
	repo             *Repository
	handlesWarmingUp []backend.Handle
}

// HandleCount returns the number of handles that are currently warming up.
func (job *WarmupJob) HandleCount() int {
	return len(job.handlesWarmingUp)
}

// Wait waits for all handles to be warm.
func (job *WarmupJob) Wait(ctx context.Context) error {
	return job.repo.be.WarmupWait(ctx, job.handlesWarmingUp)
}

// StartWarmup creates a new warmup job, requesting the backend to warmup the specified packs.
func (r *Repository) StartWarmup(ctx context.Context, packs restic.IDSet) (restic.WarmupJob, error) {
	handles := make([]backend.Handle, 0, len(packs))
	for pack := range packs {
		handles = append(
			handles,
			backend.Handle{Type: restic.PackFile, Name: pack.String()},
		)
	}
	handlesWarmingUp, err := r.be.Warmup(ctx, handles)
	return &WarmupJob{
		repo:             r,
		handlesWarmingUp: handlesWarmingUp,
	}, err
}
