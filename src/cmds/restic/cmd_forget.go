package main

import (
	"encoding/hex"
	"encoding/json"
	"restic"
	"sort"
	"strings"

	"restic/errors"

	"github.com/spf13/cobra"
)

var cmdForget = &cobra.Command{
	Use:   "forget [flags] [snapshot ID] [...]",
	Short: "forget removes snapshots from the repository",
	Long: `
The "forget" command removes snapshots according to a policy. Please note that
this command really only deletes the snapshot object in the repository, which
is a reference to data stored there. In order to remove this (now unreferenced)
data after 'forget' was run successfully, see the 'prune' command. `,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runForget(forgetOptions, globalOptions, args)
	},
}

// ForgetOptions collects all options for the forget command.
type ForgetOptions struct {
	Last    int
	Hourly  int
	Daily   int
	Weekly  int
	Monthly int
	Yearly  int

	KeepTags []string

	Host  string
	Tags  []string
	Paths []string

	GroupByTags bool
	DryRun      bool
	Prune       bool
}

var forgetOptions ForgetOptions

func init() {
	cmdRoot.AddCommand(cmdForget)

	f := cmdForget.Flags()
	f.IntVarP(&forgetOptions.Last, "keep-last", "l", 0, "keep the last `n` snapshots")
	f.IntVarP(&forgetOptions.Hourly, "keep-hourly", "H", 0, "keep the last `n` hourly snapshots")
	f.IntVarP(&forgetOptions.Daily, "keep-daily", "d", 0, "keep the last `n` daily snapshots")
	f.IntVarP(&forgetOptions.Weekly, "keep-weekly", "w", 0, "keep the last `n` weekly snapshots")
	f.IntVarP(&forgetOptions.Monthly, "keep-monthly", "m", 0, "keep the last `n` monthly snapshots")
	f.IntVarP(&forgetOptions.Yearly, "keep-yearly", "y", 0, "keep the last `n` yearly snapshots")

	f.StringSliceVar(&forgetOptions.KeepTags, "keep-tag", []string{}, "keep snapshots with this `tag` (can be specified multiple times)")
	f.BoolVarP(&forgetOptions.GroupByTags, "group-by-tags", "G", false, "Group by host,paths,tags instead of just host,paths")
	// Sadly the commonly used shortcut `H` is already used.
	f.StringVar(&forgetOptions.Host, "host", "", "only consider snapshots with the given `host`")
	// Deprecated since 2017-03-07.
	f.StringVar(&forgetOptions.Host, "hostname", "", "only consider snapshots with the given `hostname` (deprecated)")
	f.StringSliceVar(&forgetOptions.Tags, "tag", nil, "only consider snapshots which include this `tag` (can be specified multiple times)")
	f.StringSliceVar(&forgetOptions.Paths, "path", nil, "only consider snapshots which include this (absolute) `path` (can be specified multiple times)")

	f.BoolVarP(&forgetOptions.DryRun, "dry-run", "n", false, "do not delete anything, just print what would be done")
	f.BoolVar(&forgetOptions.Prune, "prune", false, "automatically run the 'prune' command if snapshots have been removed")
}

func runForget(opts ForgetOptions, gopts GlobalOptions, args []string) error {
	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	lock, err := lockRepoExclusive(repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	// Process all snapshot IDs given as arguments.
	if len(args) != 0 {
		for _, s := range args {
			// Parse argument as hex string.
			if _, err := hex.DecodeString(s); err != nil {
				Warnf("argument %q is not a snapshot ID, ignoring\n", s)
				continue
			}
			id, err := restic.FindSnapshot(repo, s)
			if err != nil {
				Warnf("could not find a snapshot for ID %q, ignoring\n", s)
				continue
			}

			if !opts.DryRun {
				h := restic.Handle{Type: restic.SnapshotFile, Name: id.String()}
				err = repo.Backend().Remove(h)
				if err != nil {
					return err
				}

				Verbosef("removed snapshot %v\n", id.Str())
			} else {
				Verbosef("would remove snapshot %v\n", id.Str())
			}
		}
		return nil
	}

	policy := restic.ExpirePolicy{
		Last:    opts.Last,
		Hourly:  opts.Hourly,
		Daily:   opts.Daily,
		Weekly:  opts.Weekly,
		Monthly: opts.Monthly,
		Yearly:  opts.Yearly,
		Tags:    opts.KeepTags,
	}

	snapshots, err := restic.LoadAllSnapshots(repo)
	if err != nil {
		return err
	}

	// Group snapshots by hostname and dirs.
	type key struct {
		Hostname string
		Paths    []string
		Tags     []string
	}

	snapshotGroups := make(map[string]restic.Snapshots)

	for _, sn := range snapshots {
		if opts.Host != "" && sn.Hostname != opts.Host {
			continue
		}

		if !sn.HasTags(opts.Tags) {
			continue
		}

		if !sn.HasPaths(opts.Paths) {
			continue
		}

		var tags []string
		if opts.GroupByTags {
			sort.StringSlice(sn.Tags).Sort()
			tags = sn.Tags
		}
		sort.StringSlice(sn.Paths).Sort()
		k, _ := json.Marshal(key{Hostname: sn.Hostname, Tags: tags, Paths: sn.Paths})
		snapshotGroups[string(k)] = append(snapshotGroups[string(k)], sn)
	}
	if len(snapshotGroups) == 0 {
		return errors.Fatal("no snapshots remained after filtering")
	}
	if policy.Empty() {
		Verbosef("no policy was specified, no snapshots will be removed\n")
	}

	removeSnapshots := 0
	for k, snapshotGroup := range snapshotGroups {
		var key key
		json.Unmarshal([]byte(k), &key)
		if opts.GroupByTags {
			Printf("snapshots for host %v, tags [%v], paths: [%v]:\n\n", key.Hostname, strings.Join(key.Tags, ", "), strings.Join(key.Paths, ", "))
		} else {
			Printf("snapshots for host %v, paths: [%v]:\n\n", key.Hostname, strings.Join(key.Paths, ", "))
		}
		keep, remove := restic.ApplyPolicy(snapshotGroup, policy)

		if len(keep) != 0 {
			Printf("keep %d snapshots:\n", len(keep))
			PrintSnapshots(globalOptions.stdout, keep)
			Printf("\n")
		}

		if len(remove) != 0 {
			Printf("remove %d snapshots:\n", len(remove))
			PrintSnapshots(globalOptions.stdout, remove)
			Printf("\n")
		}

		removeSnapshots += len(remove)

		if !opts.DryRun {
			for _, sn := range remove {
				h := restic.Handle{Type: restic.SnapshotFile, Name: sn.ID().String()}
				err = repo.Backend().Remove(h)
				if err != nil {
					return err
				}
			}
		}
	}

	if removeSnapshots > 0 && opts.Prune {
		Printf("%d snapshots have been removed, running prune\n", removeSnapshots)
		if !opts.DryRun {
			return pruneRepository(gopts, repo)
		}
	}

	return nil
}
