package repository

import (
	"context"
	"errors"
	"sync"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/restic"
)

type PacksWarmer struct {
	repo        restic.Repository
	packs       restic.IDSet
	packsResult map[restic.ID]error
	mu          sync.Mutex
}

// WamupPack requests the backend to warmup the specified pack file.
func (repo *Repository) WarmupPack(ctx context.Context, pack restic.ID) (bool, error) {
	return repo.be.Warmup(ctx, backend.Handle{Type: restic.PackFile, Name: pack.String()})
}

// WamupPackWait requests the backend to wait for the specified pack file to be warm.
func (repo *Repository) WarmupPackWait(ctx context.Context, pack restic.ID) error {
	return repo.be.WarmupWait(ctx, backend.Handle{Type: restic.PackFile, Name: pack.String()})
}

// NewPacksWarmer creates a new PacksWarmer instance.
func NewPacksWarmer(repo restic.Repository) *PacksWarmer {
	return &PacksWarmer{
		repo:        repo,
		packs:       restic.NewIDSet(),
		packsResult: make(map[restic.ID]error),
	}
}

// StartWarmup warms up the specified packs
// Returns:
//   - the number of packs that are warming up
//   - any error that occured when starting warmup
func (packsWarmer *PacksWarmer) StartWarmup(ctx context.Context, packs restic.IDs) (int, error) {
	warmupCount := 0
	for _, packID := range packs {
		if !packsWarmer.registerPack(packID) {
			continue
		}

		isWarm, err := packsWarmer.repo.WarmupPack(ctx, packID)
		if err != nil {
			packsWarmer.setResult(packID, err)
			return warmupCount, err
		}
		if isWarm {
			packsWarmer.setResult(packID, err)
		} else {
			warmupCount++
		}
	}
	return warmupCount, nil
}

// StartWarmup waits for the specified packs to be warm
func (packsWarmer *PacksWarmer) Wait(ctx context.Context, packID restic.ID) error {
	packErr, ok := packsWarmer.getResult(packID)
	if ok {
		return packErr
	}

	if !packsWarmer.packs.Has(packID) {
		return errors.New("PackNotWarmingUp")
	}

	err := packsWarmer.repo.WarmupPackWait(ctx, packID)
	packsWarmer.setResult(packID, err)

	return err
}

// registerPack saves a new pack as "being warming up". It returns true if it
// was already seen before.
func (packsWarmer *PacksWarmer) registerPack(packID restic.ID) bool {
	packsWarmer.mu.Lock()
	defer packsWarmer.mu.Unlock()

	if packsWarmer.packs.Has(packID) {
		return false
	}
	packsWarmer.packs.Insert(packID)
	return true
}

// getResult gets the result of a warmup.
// Returns:
// - the error returned by the warmup operation
// - true if the warmup is in a terminal state
func (packsWarmer *PacksWarmer) getResult(packID restic.ID) (error, bool) {
	packsWarmer.mu.Lock()
	defer packsWarmer.mu.Unlock()

	packResult, ok := packsWarmer.packsResult[packID]
	if ok {
		return packResult, true
	}

	return nil, false
}

// setResult sets the result of a warmup.
func (packsWarmer *PacksWarmer) setResult(packID restic.ID, err error) {
	packsWarmer.mu.Lock()
	defer packsWarmer.mu.Unlock()

	packsWarmer.packsResult[packID] = err
}
