package main

import (
	"context"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/spf13/cobra"
)

func newDescriptionCommand(gopts *global.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "description snapshotID description",
		Short: "Modify the description of snapshots",
		Long: `
The "description" command allows you to modify the description on a existing snapshot.

The special snapshotID "latest" can be used to restore the latest snapshot in the
repository.

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
Exit status is 10 if the repository does not exist.
Exit status is 11 if the repository is already locked.
Exit status is 12 if the password is incorrect.
`,
		GroupID:           cmdGroupDefault,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDescription(cmd.Context(), *gopts, args)
		},
	}

	return cmd
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

func runDescription(ctx context.Context, gopts global.Options, args []string) error {

	printer := ui.NewProgressPrinter(gopts.JSON, gopts.Verbosity, gopts.Term)

	printer.V("create exclusive lock for repository\n")
	ctx, repo, unlock, err := openWithExclusiveLock(ctx, gopts, false, printer)
	if err != nil {
		return err
	}
	defer unlock()

	// TODO output, summary usw.

	// check arguments
	switch {
	case len(args) < 2:
		return errors.Fatal("no snapshot ID or description specified")
	case len(args) > 2:
		return errors.Fatalf("more than one snapshot ID specified: %v", args[1:])
	}

	for sn := range FindFilteredSnapshots(ctx, repo, repo, &data.SnapshotFilter{}, args[1:], printer) {
		err := changeDescription(ctx, repo, sn, args[0])
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
