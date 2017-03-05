package main

import (
	"github.com/spf13/cobra"

	"restic"
	"restic/debug"
	"restic/errors"
	"restic/repository"
)

var cmdTag = &cobra.Command{
	Use:   "tag [flags] [snapshot-ID ...]",
	Short: "modifies tags on snapshots",
	Long: `
The "tag" command allows you to modify tags on exiting snapshots.

You can either set/replace the entire set of tags on a snapshot, or
add tags to/remove tags from the existing set.

When no snapshot-ID is given, all snapshots matching the host, tag and path filter criteria are modified.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTag(tagOptions, globalOptions, args)
	},
}

// TagOptions bundles all options for the 'tag' command.
type TagOptions struct {
	Host       string
	Paths      []string
	Tags       []string
	SetTags    []string
	AddTags    []string
	RemoveTags []string
}

var tagOptions TagOptions

func init() {
	cmdRoot.AddCommand(cmdTag)

	tagFlags := cmdTag.Flags()
	tagFlags.StringSliceVar(&tagOptions.SetTags, "set", nil, "`tag` which will replace the existing tags (can be given multiple times)")
	tagFlags.StringSliceVar(&tagOptions.AddTags, "add", nil, "`tag` which will be added to the existing tags (can be given multiple times)")
	tagFlags.StringSliceVar(&tagOptions.RemoveTags, "remove", nil, "`tag` which will be removed from the existing tags (can be given multiple times)")

	tagFlags.StringVarP(&tagOptions.Host, "host", "H", "", `only consider snapshots for this host, when no snapshot ID is given`)
	tagFlags.StringSliceVar(&tagOptions.Tags, "tag", nil, "only consider snapshots which include this `tag`, when no snapshot-ID is given")
	tagFlags.StringSliceVar(&tagOptions.Paths, "path", nil, "only consider snapshots which include this (absolute) `path`, when no snapshot-ID is given")
}

func changeTags(repo *repository.Repository, snapshotID restic.ID, setTags, addTags, removeTags, tags, paths []string, host string) (bool, error) {
	var changed bool

	sn, err := restic.LoadSnapshot(repo, snapshotID)
	if err != nil {
		return false, err
	}
	if (host != "" && host != sn.Hostname) || !sn.HasTags(tags) || !restic.SamePaths(sn.Paths, paths) {
		return false, nil
	}

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
		// Save the new snapshot.
		id, err := repo.SaveJSONUnpacked(restic.SnapshotFile, sn)
		if err != nil {
			return false, err
		}

		debug.Log("new snapshot saved as %v", id.Str())

		if err = repo.Flush(); err != nil {
			return false, err
		}

		// Remove the old snapshot.
		h := restic.Handle{Type: restic.SnapshotFile, Name: sn.ID().String()}
		if err = repo.Backend().Remove(h); err != nil {
			return false, err
		}

		debug.Log("old snapshot %v removed", sn.ID())
	}
	return changed, nil
}

func runTag(opts TagOptions, gopts GlobalOptions, args []string) error {
	if len(opts.SetTags) == 0 && len(opts.AddTags) == 0 && len(opts.RemoveTags) == 0 {
		return errors.Fatal("nothing to do!")
	}
	if len(opts.SetTags) != 0 && (len(opts.AddTags) != 0 || len(opts.RemoveTags) != 0) {
		return errors.Fatal("--set and --add/--remove cannot be given at the same time")
	}

	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	if !gopts.NoLock {
		Verbosef("Create exclusive lock for repository\n")
		lock, err := lockRepoExclusive(repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	var ids restic.IDs
	if len(args) != 0 {
		// When explit snapshot-IDs are given, the filtering does not matter anymore.
		opts.Host = ""
		opts.Tags = nil
		opts.Paths = nil

		// Process all snapshot IDs given as arguments.
		for _, s := range args {
			snapshotID, err := restic.FindSnapshot(repo, s)
			if err != nil {
				Warnf("could not find a snapshot for ID %q, ignoring: %v\n", s, err)
				continue
			}
			ids = append(ids, snapshotID)
		}
		ids = ids.Uniq()
	} else {
		// If there were no arguments, just get all snapshots.
		done := make(chan struct{})
		defer close(done)
		for snapshotID := range repo.List(restic.SnapshotFile, done) {
			ids = append(ids, snapshotID)
		}
	}

	changeCnt := 0
	for _, id := range ids {
		changed, err := changeTags(repo, id, opts.SetTags, opts.AddTags, opts.RemoveTags, opts.Tags, opts.Paths, opts.Host)
		if err != nil {
			Warnf("unable to modify the tags for snapshot ID %q, ignoring: %v\n", id, err)
			continue
		}
		if changed {
			changeCnt++
		}
	}
	if changeCnt == 0 {
		Verbosef("No snapshots were modified\n")
	} else {
		Verbosef("Modified tags on %v snapshots\n", changeCnt)
	}
	return nil
}
