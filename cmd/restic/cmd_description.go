package main

import (
	"context"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/termstatus"
	"github.com/spf13/cobra"
)

func newDescriptionCommand() *cobra.Command {
	return nil
}

type DescriptionOptions struct {
	restic.SnapshotFilter
	Description string
}

func changeDescription(ctx context.Context, repo *repository.Repository, sn *restic.Snapshot, newDescription string) error {
	sn.Description = newDescription

	sn.Original = sn.ID()
	id, err := restic.SaveSnapshot(ctx, repo, sn)
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

func runDescription(ctx context.Context, opts DescriptionOptions, gopts GlobalOptions, term *termstatus.Terminal, args []string) error {

	Verbosef("create exclusive lock for repository\n")
	ctx, repo, unlock, err := openWithExclusiveLock(ctx, gopts, false)
	if err != nil {
		return err
	}
	defer unlock()

	// TODO output, summary usw.
	for sn := range FindFilteredSnapshots(ctx, repo, repo, &opts.SnapshotFilter, args) {
		err := changeDescription(ctx, repo, sn, opts.Description)
		if err != nil {
			Warnf("unable to modify the description for snapshot ID %q, ignoring: %v'\n", sn.ID(), err)
			continue
		}
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	return nil
}
