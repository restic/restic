package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/textfile"
	"github.com/restic/restic/internal/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func newDescriptionCommand(gopts *global.Options) *cobra.Command {
	var opts changeDescriptionOptions

	cmd := &cobra.Command{
		Use:   "description snapshotID [--description description | --description-file description]",
		Short: "View or modify the description of snapshots",
		Long: `
The "description" command allows you to view or modify the description on an existing snapshot.

The special snapshotID "latest" can be used to refer to the latest snapshot in the
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
	if !opts.removeDescription && len(opts.Description) == 0 && len(opts.DescriptionFile) == 0 {
		return errors.Fatal("please specify one of --remove-description, --description or --description-file")
	}
	return opts.descriptionOptions.Check()
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

const maxDescriptionLength = 4096

var descriptionTooLargeErr = errors.New(fmt.Sprintf("The provided descriptions exceeds the maximum length of %d bytes.", maxDescriptionLength))

// readDescription returns the description text specified by either the
// `--description` option or the content of the `--description-file`
func readDescription(opts descriptionOptions) (string, error) {
	description := ""
	if len(opts.Description) > 0 {
		description = opts.Description
	} else if len(opts.DescriptionFile) > 0 {
		// Read snapshot description from file
		data, err := textfile.Read(opts.DescriptionFile)
		if err != nil {
			return "", err
		}
		descriptionScanner := bufio.NewScanner(bytes.NewReader(data))
		var builder strings.Builder
		for descriptionScanner.Scan() {
			fmt.Fprintln(&builder, descriptionScanner.Text())
		}
		description, _ = strings.CutSuffix(builder.String(), "\n")
	}

	if len(description) > maxDescriptionLength {
		return "", descriptionTooLargeErr
	}

	return description, nil
}

func changeDescription(ctx context.Context, repo *repository.Repository, sn *data.Snapshot, newDescription string, printFunc func(changedSnapshot), msg *ui.Message) error {
	if sn.Description == newDescription {
		// No need to create a new snapshot
		msg.V("description did not change")
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
	printFunc(changedSnapshot{MessageType: "changed", OldSnapshotID: *sn.ID(), NewSnapshotID: id})

	return nil
}

func runDescription(ctx context.Context, opts changeDescriptionOptions, gopts global.Options, args []string) error {

	printer := ui.NewProgressPrinter(gopts.JSON, gopts.Verbosity, gopts.Term)

	// check arguments
	if len(args) < 1 {
		return errors.Fatal("no snapshot ID specified")
	}
	opts.Check()

	descriptionChange := len(opts.Description) > 0 || len(opts.DescriptionFile) > 0
	lockExclusive := opts.removeDescription || descriptionChange

	var repo *repository.Repository
	var unlock func()
	var err error

	if lockExclusive {
		printer.V("create exclusive lock for repository")
		ctx, repo, unlock, err = openWithExclusiveLock(ctx, gopts, false, printer)
	} else {
		ctx, repo, unlock, err = openWithReadLock(ctx, gopts, gopts.NoLock, printer)
	}
	if err != nil {
		return err
	}
	defer unlock()

	printFunc := func(c changedSnapshot) {
		printer.V("old snapshot ID: %v -> new snapshot ID: %v\n", c.OldSnapshotID, c.NewSnapshotID)
	}

	if gopts.JSON {
		printFunc = func(c changedSnapshot) {
			printer.P(ui.ToJSONString(c))
		}
	}

	changeMsg := ui.NewMessage(gopts.Term, gopts.Verbosity)

	if opts.removeDescription {
		for sn := range FindFilteredSnapshots(ctx, repo, repo, &data.SnapshotFilter{}, args, printer) {
			err := changeDescription(ctx, repo, sn, "", printFunc, changeMsg)
			if err != nil {
				printer.S("unable to remove the description of snapshot ID %q, ignoring: %v'\n", sn.ID(), err)
				continue
			}
		}
	} else if descriptionChange {
		description, err := readDescription(opts.descriptionOptions)
		if err != nil {
			return err
		}
		// New description provided -> change description
		for sn := range FindFilteredSnapshots(ctx, repo, repo, &data.SnapshotFilter{}, args, printer) {
			err := changeDescription(ctx, repo, sn, description, printFunc, changeMsg)
			if err != nil {
				printer.S("unable to modify the description for snapshot ID %s, ignoring: %v'\n", sn.ID().Str(), err)
				continue
			}
		}
	}

	return ctx.Err()
}
