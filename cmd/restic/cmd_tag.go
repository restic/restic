package main

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
)

func newTagCommand() *cobra.Command {
	var opts TagOptions

	cmd := &cobra.Command{
		Use:   "tag [flags] [snapshotID ...]",
		Short: "Modify tags on snapshots",
		Long: `
The "tag" command allows you to modify tags on exiting snapshots.

You can either set/replace the entire set of tags on a snapshot, or
add tags to/remove tags from the existing set.

When no snapshotID is given, all snapshots matching the host, tag and path filter criteria are modified.

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
			term, cancel := setupTermstatus()
			defer cancel()
			return runTag(cmd.Context(), opts, globalOptions, term, args)
		},
	}

	opts.AddFlags(cmd.Flags())
	return cmd
}

// TagOptions bundles all options for the 'tag' command.
type TagOptions struct {
	restic.SnapshotFilter
	SetTags    restic.TagLists
	AddTags    restic.TagLists
	RemoveTags restic.TagLists
}

func (opts *TagOptions) AddFlags(f *pflag.FlagSet) {
	f.Var(&opts.SetTags, "set", "`tags` which will replace the existing tags in the format `tag[,tag,...]` (can be given multiple times)")
	f.Var(&opts.AddTags, "add", "`tags` which will be added to the existing tags in the format `tag[,tag,...]` (can be given multiple times)")
	f.Var(&opts.RemoveTags, "remove", "`tags` which will be removed from the existing tags in the format `tag[,tag,...]` (can be given multiple times)")
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

func changeTags(ctx context.Context, repo *repository.Repository, sn *restic.Snapshot, setTags, addTags, removeTags []string, printFunc func(changedSnapshot)) (bool, error) {
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
		id, err := restic.SaveSnapshot(ctx, repo, sn)
		if err != nil {
			return false, err
		}

		debug.Log("old snapshot %v saved as a new snapshot %v", sn.ID(), id)

		// Remove the old snapshot.
		if err = repo.RemoveUnpacked(ctx, restic.WriteableSnapshotFile, *sn.ID()); err != nil {
			return false, err
		}

		debug.Log("old snapshot %v removed", sn.ID())

		printFunc(changedSnapshot{MessageType: "changed", OldSnapshotID: *sn.ID(), NewSnapshotID: id})
	}
	return changed, nil
}

func runTag(ctx context.Context, opts TagOptions, gopts GlobalOptions, term ui.Terminal, args []string) error {
	printer := newTerminalProgressPrinter(gopts.JSON, gopts.verbosity, term)

	if len(opts.SetTags) == 0 && len(opts.AddTags) == 0 && len(opts.RemoveTags) == 0 {
		return errors.Fatal("nothing to do!")
	}
	if len(opts.SetTags) != 0 && (len(opts.AddTags) != 0 || len(opts.RemoveTags) != 0) {
		return errors.Fatal("--set and --add/--remove cannot be given at the same time")
	}

	printer.P("create exclusive lock for repository")
	ctx, repo, unlock, err := openWithExclusiveLock(ctx, gopts, false, printer)
	if err != nil {
		return err
	}
	defer unlock()

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

	for sn := range FindFilteredSnapshots(ctx, repo, repo, &opts.SnapshotFilter, args, printer) {
		changed, err := changeTags(ctx, repo, sn, opts.SetTags.Flatten(), opts.AddTags.Flatten(), opts.RemoveTags.Flatten(), printFunc)
		if err != nil {
			printer.E("unable to modify the tags for snapshot ID %q, ignoring: %v", sn.ID(), err)
			continue
		}
		if changed {
			summary.ChangedSnapshots++
		}
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	printSummary(summary)

	return nil
}
