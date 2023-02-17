package main

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
)

var cmdTag = &cobra.Command{
	Use:   "tag [flags] [snapshot-ID ...]",
	Short: "Modify tags on snapshots",
	Long: `
The "tag" command allows you to modify tags on exiting snapshots.

You can either set/replace the entire set of tags on a snapshot, or
add tags to/remove tags from the existing set.

When no snapshot-ID is given, all snapshots matching the host, tag and path filter criteria are modified.

EXIT STATUS
===========

Exit status is 0 if the command was successful, and non-zero if there was any error.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTag(cmd.Context(), tagOptions, globalOptions, args)
	},
}

// TagOptions bundles all options for the 'tag' command.
type TagOptions struct {
	restic.SnapshotFilter
	SetTags    restic.TagLists
	AddTags    restic.TagLists
	RemoveTags restic.TagLists
}

var tagOptions TagOptions

func init() {
	cmdRoot.AddCommand(cmdTag)

	tagFlags := cmdTag.Flags()
	tagFlags.Var(&tagOptions.SetTags, "set", "`tags` which will replace the existing tags in the format `tag[,tag,...]` (can be given multiple times)")
	tagFlags.Var(&tagOptions.AddTags, "add", "`tags` which will be added to the existing tags in the format `tag[,tag,...]` (can be given multiple times)")
	tagFlags.Var(&tagOptions.RemoveTags, "remove", "`tags` which will be removed from the existing tags in the format `tag[,tag,...]` (can be given multiple times)")
	initMultiSnapshotFilter(tagFlags, &tagOptions.SnapshotFilter, true)
}

func changeTags(ctx context.Context, repo *repository.Repository, sn *restic.Snapshot, setTags, addTags, removeTags []string) (bool, error) {
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

		debug.Log("new snapshot saved as %v", id)

		// Remove the old snapshot.
		h := restic.Handle{Type: restic.SnapshotFile, Name: sn.ID().String()}
		if err = repo.Backend().Remove(ctx, h); err != nil {
			return false, err
		}

		debug.Log("old snapshot %v removed", sn.ID())
	}
	return changed, nil
}

func runTag(ctx context.Context, opts TagOptions, gopts GlobalOptions, args []string) error {
	if len(opts.SetTags) == 0 && len(opts.AddTags) == 0 && len(opts.RemoveTags) == 0 {
		return errors.Fatal("nothing to do!")
	}
	if len(opts.SetTags) != 0 && (len(opts.AddTags) != 0 || len(opts.RemoveTags) != 0) {
		return errors.Fatal("--set and --add/--remove cannot be given at the same time")
	}

	repo, err := OpenRepository(ctx, gopts)
	if err != nil {
		return err
	}

	if !gopts.NoLock {
		Verbosef("create exclusive lock for repository\n")
		var lock *restic.Lock
		lock, ctx, err = lockRepoExclusive(ctx, repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	changeCnt := 0
	for sn := range FindFilteredSnapshots(ctx, repo.Backend(), repo, &opts.SnapshotFilter, args) {
		changed, err := changeTags(ctx, repo, sn, opts.SetTags.Flatten(), opts.AddTags.Flatten(), opts.RemoveTags.Flatten())
		if err != nil {
			Warnf("unable to modify the tags for snapshot ID %q, ignoring: %v\n", sn.ID(), err)
			continue
		}
		if changed {
			changeCnt++
		}
	}
	if changeCnt == 0 {
		Verbosef("no snapshots were modified\n")
	} else {
		Verbosef("modified tags on %v snapshots\n", changeCnt)
	}
	return nil
}
