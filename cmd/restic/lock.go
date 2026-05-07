package main

import (
	"context"

	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/tracing"
	"github.com/restic/restic/internal/ui/progress"
)

func internalOpenWithLocked(ctx context.Context, gopts global.Options, dryRun bool, exclusive bool, printer progress.Printer) (context.Context, *repository.Repository, func(), error) {
	openCtx, openSpan := tracing.Tracer().Start(ctx, "restic.open_repository")
	repo, err := global.OpenRepository(openCtx, gopts, printer)
	if err != nil {
		tracing.EndSpanWithError(openSpan, err)
		return nil, nil, nil, err
	}

	unlock := func() {}
	if !dryRun {
		var lock *repository.Unlocker

		lock, ctx, err = repository.Lock(openCtx, repo, exclusive, gopts.RetryLock, func(msg string) {
			if !gopts.JSON {
				printer.P("%s", msg)
			}
		}, printer.E)
		if err != nil {
			tracing.EndSpanWithError(openSpan, err)
			return nil, nil, nil, err
		}

		unlock = lock.Unlock
	} else {
		repo.SetDryRun()
		ctx = openCtx
	}

	tracing.EndSpanWithError(openSpan, nil)
	return ctx, repo, unlock, nil
}

func openWithReadLock(ctx context.Context, gopts global.Options, noLock bool, printer progress.Printer) (context.Context, *repository.Repository, func(), error) {
	// TODO enforce read-only operations once the locking code has moved to the repository
	return internalOpenWithLocked(ctx, gopts, noLock, false, printer)
}

func openWithAppendLock(ctx context.Context, gopts global.Options, dryRun bool, printer progress.Printer) (context.Context, *repository.Repository, func(), error) {
	// TODO enforce non-exclusive operations once the locking code has moved to the repository
	return internalOpenWithLocked(ctx, gopts, dryRun, false, printer)
}

func openWithExclusiveLock(ctx context.Context, gopts global.Options, dryRun bool, printer progress.Printer) (context.Context, *repository.Repository, func(), error) {
	return internalOpenWithLocked(ctx, gopts, dryRun, true, printer)
}
