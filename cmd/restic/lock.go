package main

import (
	"context"

	"github.com/restic/restic/internal/repository"
)

func internalOpenWithLocked(ctx context.Context, gopts GlobalOptions, dryRun bool, exclusive bool) (context.Context, *repository.Repository, func(), error) {
	repo, err := OpenRepository(ctx, gopts)
	if err != nil {
		return nil, nil, nil, err
	}

	unlock := func() {}
	if !dryRun {
		var lock *repository.Unlocker

		lock, ctx, err = repository.Lock(ctx, repo, exclusive, gopts.RetryLock, func(msg string) {
			if !gopts.JSON {
				Verbosef("%s", msg)
			}
		}, Warnf)

		unlock = lock.Unlock
		// make sure that a repository is unlocked properly and after cancel() was
		// called by the cleanup handler in global.go
		AddCleanupHandler(func(code int) (int, error) {
			lock.Unlock()
			return code, nil
		})

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
