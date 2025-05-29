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
	"github.com/spf13/pflag"
)

func newDescriptionCommand(gopts *global.Options) *cobra.Command {
	var opts changeDescriptionOptions

	cmd := &cobra.Command{
		Use:   "description snapshotID [--set description | --set-file description]",
		Short: "Modify the description of snapshots",
		Long: `
The "description" command allows you to modify the description on an existing snapshot.

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
			return runDescription(cmd.Context(), opts, *gopts, args)
		},
	}

	opts.AddFlags(cmd.Flags())
	return cmd
}

type changeDescriptionOptions struct {
	descriptionOptions
	removeDescription bool
}

func (opts *changeDescriptionOptions) AddFlags(f *pflag.FlagSet) {
	f.BoolVar(&opts.removeDescription, "remove-description", false, "remove the description from a snapshot")
	opts.descriptionOptions.AddFlags(f)
}

func (opts *changeDescriptionOptions) Check() error {
	err := opts.descriptionOptions.Check()
	if err != nil {
		return err
	}
	return nil
}

type descriptionOptions struct {
	Description     string
	DescriptionFile string
}

func (opts *descriptionOptions) AddFlags(f *pflag.FlagSet) {
	f.StringVar(&opts.Description, "description", "", "set the description of this snapshot")
	f.StringVar(&opts.DescriptionFile, "description-file", "", "set the description of this snapshot to the content of the file")
}

func (opts *descriptionOptions) Check() error {
	if len(opts.Description) > 0 && len(opts.DescriptionFile) > 0 {
		return errors.Fatal("--description and --description-file cannot be used together")
	}

	return nil
}

func changeDescription(ctx context.Context, repo *repository.Repository, sn *data.Snapshot, newDescription string) error {
	if sn.Description == newDescription {
		// No need to create a new snapshot
		return nil
	}

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

func runDescription(ctx context.Context, opts changeDescriptionOptions, gopts global.Options, args []string) error {

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
	case len(args) < 1:
		return errors.Fatal("no snapshot ID specified")
	case len(args) > 1:
		return errors.Fatalf("more than one snapshot ID specified: %v", args)
	}
	opts.Check()

	if opts.removeDescription {
		for sn := range FindFilteredSnapshots(ctx, repo, repo, &data.SnapshotFilter{}, args, printer) {
			err := changeDescription(ctx, repo, sn, "")
			if err != nil {
				printer.S("unable to remove the description of snapshot ID %q, ignoring: %v'\n", sn.ID(), err)
				continue
			}
		}
		return nil
	}

	if len(opts.Description) > 0 || len(opts.DescriptionFile) > 0 {
		description, err := readDescription(opts.descriptionOptions)
		if err != nil {
			return err
		}
		// New description provided -> change description
		for sn := range FindFilteredSnapshots(ctx, repo, repo, &data.SnapshotFilter{}, args, printer) {
			err := changeDescription(ctx, repo, sn, description)
			if err != nil {
				printer.S("unable to modify the description for snapshot ID %s, ignoring: %v'\n", sn.ID().Str(), err)
				continue
			}
		}
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	return nil
}
