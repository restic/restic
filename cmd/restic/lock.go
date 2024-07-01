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
		if err != nil {
			return nil, nil, nil, err
		}

		unlock = lock.Unlock
	} else {
		repo.SetDryRun()
	}

	return ctx, repo, unlock, nil
}

func openWithReadLock(ctx context.Context, gopts GlobalOptions, noLock bool) (context.Context, *repository.Repository, func(), error) {
	// TODO enforce read-only operations once the locking code has moved to the repository
	return internalOpenWithLocked(ctx, gopts, noLock, false)
}

func openWithAppendLock(ctx context.Context, gopts GlobalOptions, dryRun bool) (context.Context, *repository.Repository, func(), error) {
	// TODO enforce non-exclusive operations once the locking code has moved to the repository
	return internalOpenWithLocked(ctx, gopts, dryRun, false)
}

func openWithExclusiveLock(ctx context.Context, gopts GlobalOptions, dryRun bool) (context.Context, *repository.Repository, func(), error) {
	return internalOpenWithLocked(ctx, gopts, dryRun, true)
}
