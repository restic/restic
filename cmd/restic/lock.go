package main

import (
	"context"
	"sync"

	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
)

var globalLocks struct {
	sync.Once
}

func internalOpenWithLocked(ctx context.Context, gopts GlobalOptions, dryRun bool, exclusive bool) (context.Context, *repository.Repository, func(), error) {
	repo, err := OpenRepository(ctx, gopts)
	if err != nil {
		return nil, nil, nil, err
	}

	unlock := func() {}
	if !dryRun {
		var lock *restic.Lock

		// make sure that a repository is unlocked properly and after cancel() was
		// called by the cleanup handler in global.go
		globalLocks.Do(func() {
			AddCleanupHandler(repository.UnlockAll)
		})

		lock, ctx, err = repository.Lock(ctx, repo, exclusive, gopts.RetryLock, func(msg string) {
			if !gopts.JSON {
				Verbosef("%s", msg)
			}
		}, Warnf)
		unlock = func() {
			repository.Unlock(lock)
		}
		if err != nil {
			return nil, nil, nil, err
		}
	} else {
		repo.SetDryRun()
	}

	return ctx, repo, unlock, nil
}

func openWithReadLock(ctx context.Context, gopts GlobalOptions, noLock bool) (context.Context, *repository.Repository, func(), error) {
	// TODO enfore read-only operations once the locking code has moved to the repository
	return internalOpenWithLocked(ctx, gopts, noLock, false)
}

func openWithAppendLock(ctx context.Context, gopts GlobalOptions, dryRun bool) (context.Context, *repository.Repository, func(), error) {
	// TODO enfore non-exclusive operations once the locking code has moved to the repository
	return internalOpenWithLocked(ctx, gopts, dryRun, false)
}

func openWithExclusiveLock(ctx context.Context, gopts GlobalOptions, dryRun bool) (context.Context, *repository.Repository, func(), error) {
	return internalOpenWithLocked(ctx, gopts, dryRun, true)
}
