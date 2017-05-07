package main

import (
	"context"
	"encoding/json"
	"restic"
	"sort"
	"strings"

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
	Last     int
	Hourly   int
	Daily    int
	Weekly   int
	Monthly  int
	Yearly   int
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

	f.SortFlags = false
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

	// group by hostname and dirs
	type key struct {
		Hostname string
		Paths    []string
		Tags     []string
	}
	snapshotGroups := make(map[string]restic.Snapshots)

	ctx, cancel := context.WithCancel(gopts.ctx)
	defer cancel()
	for sn := range FindFilteredSnapshots(ctx, repo, opts.Host, opts.Tags, opts.Paths, args) {
		if len(args) > 0 {
			// When explicit snapshots args are given, remove them immediately.
			if !opts.DryRun {
				h := restic.Handle{Type: restic.SnapshotFile, Name: sn.ID().String()}
				if err = repo.Backend().Remove(h); err != nil {
					return err
				}
				Verbosef("removed snapshot %v\n", sn.ID().Str())
			} else {
				Verbosef("would have removed snapshot %v\n", sn.ID().Str())
			}
		} else {
			var tags []string
			if opts.GroupByTags {
				tags = sn.Tags
				sort.StringSlice(tags).Sort()
			}
			sort.StringSlice(sn.Paths).Sort()
			k, err := json.Marshal(key{Hostname: sn.Hostname, Tags: tags, Paths: sn.Paths})
			if err != nil {
				return err
			}
			snapshotGroups[string(k)] = append(snapshotGroups[string(k)], sn)
		}
	}
	if len(args) > 0 {
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

	if policy.Empty() {
		Verbosef("no policy was specified, no snapshots will be removed\n")
		return nil
	}

	removeSnapshots := 0
	for k, snapshotGroup := range snapshotGroups {
		var key key
		if json.Unmarshal([]byte(k), &key) != nil {
			return err
		}
		if opts.GroupByTags {
			Verbosef("snapshots for host %v, tags [%v], paths: [%v]:\n\n", key.Hostname, strings.Join(key.Tags, ", "), strings.Join(key.Paths, ", "))
		} else {
			Verbosef("snapshots for host %v, paths: [%v]:\n\n", key.Hostname, strings.Join(key.Paths, ", "))
		}
		keep, remove := restic.ApplyPolicy(snapshotGroup, policy)

		if len(keep) != 0 && !gopts.Quiet {
			Printf("keep %d snapshots:\n", len(keep))
			PrintSnapshots(globalOptions.stdout, keep)
			Printf("\n")
		}

		if len(remove) != 0 && !gopts.Quiet {
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
		Verbosef("%d snapshots have been removed, running prune\n", removeSnapshots)
		if !opts.DryRun {
			return pruneRepository(gopts, repo)
		}
	}

	return nil
}
