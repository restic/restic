package repository

import (
	"context"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/restic"
)

type warmupJob struct {
	repo             *Repository
	handlesWarmingUp []backend.Handle
}

// HandleCount returns the number of handles that are currently warming up.
func (job *warmupJob) HandleCount() int {
	return len(job.handlesWarmingUp)
}

// Wait waits for all handles to be warm.
func (job *warmupJob) Wait(ctx context.Context) error {
	return job.repo.be.WarmupWait(ctx, job.handlesWarmingUp)
}

// StartWarmup creates a new warmup job, requesting the backend to warmup the specified packs.
func (r *Repository) StartWarmup(ctx context.Context, packs restic.IDSet) (restic.WarmupJob, error) {
	handles := make([]backend.Handle, 0, len(packs))
	for pack := range packs {
		handles = append(handles, beHandle(restic.PackFile, pack.String()))
	}
	handlesWarmingUp, err := r.be.Warmup(ctx, handles)
	return &warmupJob{
		repo:             r,
		handlesWarmingUp: handlesWarmingUp,
	}, err
}
