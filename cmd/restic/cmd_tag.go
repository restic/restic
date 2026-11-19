package main

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"
)

func newTagCommand(globalOptions *global.Options) *cobra.Command {
	var opts TagOptions

	cmd := &cobra.Command{
		Use:   "tag [flags] [snapshotID ...]",
		Short: "Modify tags on snapshots",
		Long: `
The "tag" command allows you to modify tags on existing snapshots.

You can either set/replace the entire set of tags on a snapshot, or
add tags to/remove tags from the existing set.

When no snapshotID is given, all snapshots matching the host, tag and path filter criteria are modified.

By default, the original snapshots are removed after the snapshots with the
modified tags have been created, which requires an exclusive lock on the
repository. When --keep-original is given, the original snapshots are kept and
only a non-exclusive lock is acquired.

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
			finalizeSnapshotFilter(&opts.SnapshotFilter)
			return runTag(cmd.Context(), opts, *globalOptions, globalOptions.Term, args)
		},
	}

	opts.AddFlags(cmd.Flags())
	return cmd
}

// TagOptions bundles all options for the 'tag' command.
type TagOptions struct {
	data.SnapshotFilter
	SetTags      data.TagLists
	AddTags      data.TagLists
	RemoveTags   data.TagLists
	KeepOriginal bool
}

func (opts *TagOptions) AddFlags(f *pflag.FlagSet) {
	f.Var(&opts.SetTags, "set", "`tags` which will replace the existing tags in the format `tag[,tag,...]` (can be given multiple times)")
	f.Var(&opts.AddTags, "add", "`tags` which will be added to the existing tags in the format `tag[,tag,...]` (can be given multiple times)")
	f.Var(&opts.RemoveTags, "remove", "`tags` which will be removed from the existing tags in the format `tag[,tag,...]` (can be given multiple times)")
	f.BoolVar(&opts.KeepOriginal, "keep-original", false, "keep the original snapshots instead of removing them")
	initMultiSnapshotFilter(f, &opts.SnapshotFilter, true)
}

type changedSnapshot struct {
	MessageType   string    `json:"message_type"` // changed
	OldSnapshotID restic.ID `json:"old_snapshot_id"`
	NewSnapshotID restic.ID `json:"new_snapshot_id"`
}

type changedSnapshotsSummary struct {
	MessageType      string `json:"message_type"` // summary
	ChangedSnapshots int    `json:"changed_snapshots"`
}

func changeTags(ctx context.Context, repo *repository.Repository, sn *data.Snapshot, setTags, addTags, removeTags []string, keepOriginal bool, printFunc func(changedSnapshot)) (bool, error) {
	var changed bool

	if len(setTags) != 0 {
		// Setting the tag to an empty string really means no tags.
		if len(setTags) == 1 && setTags[0] == "" {
			setTags = nil
		}
		sn.Tags = setTags
		changed = true
	} else {
		changed = sn.AddTags(addTags)
		if sn.RemoveTags(removeTags) {
			changed = true
		}
	}

	if changed {
		// Retain the original snapshot id over all tag changes.
		if sn.Original == nil {
			sn.Original = sn.ID()
		}

		// Save the new snapshot.
		id, err := data.SaveSnapshot(ctx, repo, sn)
		if err != nil {
			return false, err
		}

		debug.Log("old snapshot %v saved as a new snapshot %v", sn.ID(), id)

		if !keepOriginal {
			// Remove the old snapshot.
			if err = repo.RemoveUnpacked(ctx, restic.WriteableSnapshotFile, *sn.ID()); err != nil {
				return false, err
			}

			debug.Log("old snapshot %v removed", sn.ID())
		}

		printFunc(changedSnapshot{MessageType: "changed", OldSnapshotID: *sn.ID(), NewSnapshotID: id})
	}
	return changed, nil
}

func runTag(ctx context.Context, opts TagOptions, gopts global.Options, term ui.Terminal, args []string) error {
	printer := progress.NewTerminalPrinter(gopts.JSON, gopts.Verbosity, term)

	if len(opts.SetTags) == 0 && len(opts.AddTags) == 0 && len(opts.RemoveTags) == 0 {
		return errors.Fatal("nothing to do!")
	}
	if len(opts.SetTags) != 0 && (len(opts.AddTags) != 0 || len(opts.RemoveTags) != 0) {
		return errors.Fatal("--set and --add/--remove cannot be given at the same time")
	}

	var (
		repo   *repository.Repository
		unlock func()
		err    error
	)

	if opts.KeepOriginal {
		ctx, repo, unlock, err = openWithAppendLock(ctx, gopts, false, printer)
	} else {
		printer.P("create exclusive lock for repository")
		ctx, repo, unlock, err = openWithExclusiveLock(ctx, gopts, false, printer)
	}
	if err != nil {
		return err
	}
	defer unlock()

	// Memorize the snapshot list so that newly created snapshots are not processed again.
	snapshotLister, err := restic.MemorizeList(ctx, repo, restic.SnapshotFile)
	if err != nil {
		return err
	}

	printFunc := func(c changedSnapshot) {
		printer.V("old snapshot ID: %v -> new snapshot ID: %v", c.OldSnapshotID, c.NewSnapshotID)
	}

	summary := changedSnapshotsSummary{MessageType: "summary", ChangedSnapshots: 0}
	printSummary := func(c changedSnapshotsSummary) {
		if c.ChangedSnapshots == 0 {
			printer.P("no snapshots were modified")
		} else {
			printer.P("modified %v snapshots", c.ChangedSnapshots)
		}
	}

	if gopts.JSON {
		printFunc = func(c changedSnapshot) {
			term.Print(ui.ToJSONString(c))
		}
		printSummary = func(c changedSnapshotsSummary) {
			term.Print(ui.ToJSONString(c))
		}
	}

	err = opts.SnapshotFilter.FindAll(ctx, snapshotLister, repo, args, func(_ string, sn *data.Snapshot, err error) error {
		if err != nil {
			return err
		}
		changed, err := changeTags(ctx, repo, sn, opts.SetTags.Flatten(), opts.AddTags.Flatten(), opts.RemoveTags.Flatten(), opts.KeepOriginal, printFunc)
		if err != nil {
			printer.E("unable to modify the tags for snapshot ID %q, ignoring: %v", sn.ID(), err)
			return nil
		}
		if changed {
			summary.ChangedSnapshots++
		}
		return nil
	})
	if err != nil {
		return err
	}

	printSummary(summary)

	return nil
}
