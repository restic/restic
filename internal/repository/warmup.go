package repository

import (
	"context"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/restic"
)

func (repo *Repository) handlesFromPacks(packs restic.IDSet) []backend.Handle {
	handles := make([]backend.Handle, 0, len(packs))
	for pack := range packs {
		handles = append(
			handles,
			backend.Handle{Type: restic.PackFile, Name: pack.String()},
		)
	}
	return handles
}

// WamupPacks requests the backend to warmup the specified packs.
func (repo *Repository) WarmupPacks(ctx context.Context, packs restic.IDSet) (int, error) {
	return repo.be.Warmup(ctx, repo.handlesFromPacks(packs))
}

// WamupPacksWait requests the backend to wait for the specified packs to be warm.
func (repo *Repository) WarmupPacksWait(ctx context.Context, packs restic.IDSet) error {
	return repo.be.WarmupWait(ctx, repo.handlesFromPacks(packs))
}
