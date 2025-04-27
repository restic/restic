package main

import (
	"context"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/spf13/cobra"
)

func newDescriptionCommand() *cobra.Command {
	return nil
}

type DescriptionOptions struct {
	data.SnapshotFilter
	Description string
}

func changeDescription(ctx context.Context, repo *repository.Repository, sn *data.Snapshot, newDescription string) error {
	sn.Description = newDescription

	sn.Original = sn.ID()
	id, err := data.SaveSnapshot(ctx, repo, sn)
	if err != nil {
		return err
	}

	debug.Log("old snapshot %v saved as new snapshot %v", sn.ID(), id)

	if err = repo.RemoveUnpacked(ctx, restic.WriteableSnapshotFile, *sn.ID()); err != nil {
		return err
	}

	debug.Log("old snapshot %v removed", sn.ID())

	return nil
}

func runDescription(ctx context.Context, opts DescriptionOptions, gopts global.Options, args []string) error {

	printer := ui.NewProgressPrinter(gopts.JSON, gopts.Verbosity, gopts.Term)

	printer.V("create exclusive lock for repository\n")
	ctx, repo, unlock, err := openWithExclusiveLock(ctx, gopts, false, printer)
	if err != nil {
		return err
	}
	defer unlock()

	// TODO output, summary usw.
	for sn := range FindFilteredSnapshots(ctx, repo, repo, &opts.SnapshotFilter, args, printer) {
		err := changeDescription(ctx, repo, sn, opts.Description)
		if err != nil {
			printer.S("unable to modify the description for snapshot ID %q, ignoring: %v'\n", sn.ID(), err)
			continue
		}
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	return nil
}
