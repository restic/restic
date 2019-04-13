package main

import (
	"context"
	"encoding/json"
	"io"
	"sort"
	"strings"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	"github.com/spf13/cobra"
)

var cmdForget = &cobra.Command{
	Use:   "forget [flags] [snapshot ID] [...]",
	Short: "Remove snapshots from the repository",
	Long: `
The "forget" command removes snapshots according to a policy. Please note that
this command really only deletes the snapshot object in the repository, which
is a reference to data stored there. In order to remove this (now unreferenced)
data after 'forget' was run successfully, see the 'prune' command. `,
	DisableAutoGenTag: true,
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
	Within   restic.Duration
	KeepTags restic.TagLists

	Host    string
	Tags    restic.TagLists
	Paths   []string
	Compact bool

	// Grouping
	GroupBy string
	DryRun  bool
	Prune   bool
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
	f.VarP(&forgetOptions.Within, "keep-within", "", "keep snapshots that are newer than `duration` (eg. 1y5m7d2h) relative to the latest snapshot")

	f.Var(&forgetOptions.KeepTags, "keep-tag", "keep snapshots with this `taglist` (can be specified multiple times)")
	f.StringVar(&forgetOptions.Host, "host", "", "only consider snapshots with the given `host`")
	f.StringVar(&forgetOptions.Host, "hostname", "", "only consider snapshots with the given `hostname`")
	f.MarkDeprecated("hostname", "use --host")

	f.Var(&forgetOptions.Tags, "tag", "only consider snapshots which include this `taglist` in the format `tag[,tag,...]` (can be specified multiple times)")

	f.StringArrayVar(&forgetOptions.Paths, "path", nil, "only consider snapshots which include this (absolute) `path` (can be specified multiple times)")
	f.BoolVarP(&forgetOptions.Compact, "compact", "c", false, "use compact format")

	f.StringVarP(&forgetOptions.GroupBy, "group-by", "g", "host,paths", "string for grouping snapshots by host,paths,tags")
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

	var GroupByTag bool
	var GroupByHost bool
	var GroupByPath bool
	var GroupOptionList []string

	GroupOptionList = strings.Split(opts.GroupBy, ",")

	for _, option := range GroupOptionList {
		switch option {
		case "host":
			GroupByHost = true
		case "paths":
			GroupByPath = true
		case "tags":
			GroupByTag = true
		case "":
		default:
			return errors.Fatal("unknown grouping option: '" + option + "'")
		}
	}

	removeSnapshots := 0

	ctx, cancel := context.WithCancel(gopts.ctx)
	defer cancel()
	for sn := range FindFilteredSnapshots(ctx, repo, opts.Host, opts.Tags, opts.Paths, args) {
		if len(args) > 0 {
			// When explicit snapshots args are given, remove them immediately.
			if !opts.DryRun {
				h := restic.Handle{Type: restic.SnapshotFile, Name: sn.ID().String()}
				if err = repo.Backend().Remove(gopts.ctx, h); err != nil {
					return err
				}
				Verbosef("removed snapshot %v\n", sn.ID().Str())
				removeSnapshots++
			} else {
				Verbosef("would have removed snapshot %v\n", sn.ID().Str())
			}
		} else {
			// Determining grouping-keys
			var tags []string
			var hostname string
			var paths []string

			if GroupByTag {
				tags = sn.Tags
				sort.StringSlice(tags).Sort()
			}
			if GroupByHost {
				hostname = sn.Hostname
			}
			if GroupByPath {
				paths = sn.Paths
			}

			sort.StringSlice(sn.Paths).Sort()
			var k []byte
			var err error

			k, err = json.Marshal(key{Tags: tags, Hostname: hostname, Paths: paths})

			if err != nil {
				return err
			}
			snapshotGroups[string(k)] = append(snapshotGroups[string(k)], sn)
		}
	}

	policy := restic.ExpirePolicy{
		Last:    opts.Last,
		Hourly:  opts.Hourly,
		Daily:   opts.Daily,
		Weekly:  opts.Weekly,
		Monthly: opts.Monthly,
		Yearly:  opts.Yearly,
		Within:  opts.Within,
		Tags:    opts.KeepTags,
	}

	if policy.Empty() && len(args) == 0 {
		Verbosef("no policy was specified, no snapshots will be removed\n")
	}

	if !policy.Empty() {
		if !gopts.JSON {
			Verbosef("Applying Policy: %v\n", policy)
		}

		var jsonGroups []*ForgetGroup

		for k, snapshotGroup := range snapshotGroups {
			var key key
			if json.Unmarshal([]byte(k), &key) != nil {
				return err
			}

			var fg ForgetGroup
			// Info
			if !gopts.JSON {
				Verbosef("snapshots")
			}
			var infoStrings []string
			if GroupByTag {
				infoStrings = append(infoStrings, "tags ["+strings.Join(key.Tags, ", ")+"]")
				fg.Tags = key.Tags
			}
			if GroupByHost {
				infoStrings = append(infoStrings, "host ["+key.Hostname+"]")
				fg.Host = key.Hostname
			}
			if GroupByPath {
				infoStrings = append(infoStrings, "paths ["+strings.Join(key.Paths, ", ")+"]")
				fg.Paths = key.Paths
			}
			if infoStrings != nil && !gopts.JSON {
				Verbosef(" for (" + strings.Join(infoStrings, ", ") + ")")
			}
			if !gopts.JSON {
				Verbosef(":\n\n")
			}

			keep, remove, reasons := restic.ApplyPolicy(snapshotGroup, policy)

			if len(keep) != 0 && !gopts.Quiet && !gopts.JSON {
				Printf("keep %d snapshots:\n", len(keep))
				PrintSnapshots(globalOptions.stdout, keep, reasons, opts.Compact)
				Printf("\n")
			}
			addJSONSnapshots(&fg.Keep, keep)

			if len(remove) != 0 && !gopts.Quiet && !gopts.JSON {
				Printf("remove %d snapshots:\n", len(remove))
				PrintSnapshots(globalOptions.stdout, remove, nil, opts.Compact)
				Printf("\n")
			}
			addJSONSnapshots(&fg.Remove, remove)

			fg.Reasons = reasons

			jsonGroups = append(jsonGroups, &fg)

			removeSnapshots += len(remove)

			if !opts.DryRun {
				for _, sn := range remove {
					h := restic.Handle{Type: restic.SnapshotFile, Name: sn.ID().String()}
					err = repo.Backend().Remove(gopts.ctx, h)
					if err != nil {
						return err
					}
				}
			}
		}

		if gopts.JSON {
			err = printJSONForget(gopts.stdout, jsonGroups)
			if err != nil {
				return err
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

// ForgetGroup helps to print what is forgotten in JSON.
type ForgetGroup struct {
	Tags    []string            `json:"tags"`
	Host    string              `json:"host"`
	Paths   []string            `json:"paths"`
	Keep    []Snapshot          `json:"keep"`
	Remove  []Snapshot          `json:"remove"`
	Reasons []restic.KeepReason `json:"reasons"`
}

func addJSONSnapshots(js *[]Snapshot, list restic.Snapshots) {
	for _, sn := range list {
		k := Snapshot{
			Snapshot: sn,
			ID:       sn.ID(),
			ShortID:  sn.ID().Str(),
		}
		*js = append(*js, k)
	}
}

func printJSONForget(stdout io.Writer, forgets []*ForgetGroup) error {
	return json.NewEncoder(stdout).Encode(forgets)
}
