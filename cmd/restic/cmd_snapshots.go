package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/table"
	"github.com/spf13/cobra"
)

var cmdSnapshots = &cobra.Command{
	Use:   "snapshots [snapshotID ...]",
	Short: "List all snapshots",
	Long: `
The "snapshots" command lists all snapshots stored in the repository.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSnapshots(snapshotOptions, globalOptions, args)
	},
}

// SnapshotOptions bundles all options for the snapshots command.
type SnapshotOptions struct {
	Host    string
	Tags    restic.TagLists
	Paths   []string
	Compact bool
	Last    bool
	GroupBy string
}

var snapshotOptions SnapshotOptions

func init() {
	cmdRoot.AddCommand(cmdSnapshots)

	f := cmdSnapshots.Flags()
	f.StringVarP(&snapshotOptions.Host, "host", "H", "", "only consider snapshots for this `host`")
	f.Var(&snapshotOptions.Tags, "tag", "only consider snapshots which include this `taglist` (can be specified multiple times)")
	f.StringArrayVar(&snapshotOptions.Paths, "path", nil, "only consider snapshots for this `path` (can be specified multiple times)")
	f.BoolVarP(&snapshotOptions.Compact, "compact", "c", false, "use compact format")
	f.BoolVar(&snapshotOptions.Last, "last", false, "only show the last snapshot for each host and path")
	f.StringVarP(&snapshotOptions.GroupBy, "group-by", "g", "", "string for grouping snapshots by host,paths,tags")
}

type groupKey struct {
	Hostname string
	Paths    []string
	Tags     []string
}

func runSnapshots(opts SnapshotOptions, gopts GlobalOptions, args []string) error {
	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	if !gopts.NoLock {
		lock, err := lockRepo(repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	// group by hostname and dirs
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

	ctx, cancel := context.WithCancel(gopts.ctx)
	defer cancel()

	for sn := range FindFilteredSnapshots(ctx, repo, opts.Host, opts.Tags, opts.Paths, args) {
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

		k, err = json.Marshal(groupKey{Tags: tags, Hostname: hostname, Paths: paths})

		if err != nil {
			return err
		}
		snapshotGroups[string(k)] = append(snapshotGroups[string(k)], sn)
	}

	for k, list := range snapshotGroups {
		if opts.Last {
			list = FilterLastSnapshots(list)
		}
		sort.Sort(sort.Reverse(list))
		snapshotGroups[k] = list
	}

	if gopts.JSON {
		err := printSnapshotGroupJSON(gopts.stdout, snapshotGroups, GroupByTag || GroupByHost || GroupByPath)
		if err != nil {
			Warnf("error printing snapshots: %v\n", err)
		}
		return nil
	}

	for k, list := range snapshotGroups {
		err := PrintSnapshotGroupHeader(gopts.stdout, k, GroupByTag, GroupByHost, GroupByPath)
		if err != nil {
			Warnf("error printing snapshots: %v\n", err)
			return nil
		}

		PrintSnapshots(gopts.stdout, list, nil, opts.Compact)
	}

	return nil
}

// filterLastSnapshotsKey is used by FilterLastSnapshots.
type filterLastSnapshotsKey struct {
	Hostname    string
	JoinedPaths string
}

// newFilterLastSnapshotsKey initializes a filterLastSnapshotsKey from a Snapshot
func newFilterLastSnapshotsKey(sn *restic.Snapshot) filterLastSnapshotsKey {
	// Shallow slice copy
	var paths = make([]string, len(sn.Paths))
	copy(paths, sn.Paths)
	sort.Strings(paths)
	return filterLastSnapshotsKey{sn.Hostname, strings.Join(paths, "|")}
}

// FilterLastSnapshots filters a list of snapshots to only return the last
// entry for each hostname and path. If the snapshot contains multiple paths,
// they will be joined and treated as one item.
func FilterLastSnapshots(list restic.Snapshots) restic.Snapshots {
	// Sort the snapshots so that the newer ones are listed first
	sort.SliceStable(list, func(i, j int) bool {
		return list[i].Time.After(list[j].Time)
	})

	var results restic.Snapshots
	seen := make(map[filterLastSnapshotsKey]bool)
	for _, sn := range list {
		key := newFilterLastSnapshotsKey(sn)
		if !seen[key] {
			seen[key] = true
			results = append(results, sn)
		}
	}
	return results
}

// PrintSnapshots prints a text table of the snapshots in list to stdout.
func PrintSnapshots(stdout io.Writer, list restic.Snapshots, reasons []restic.KeepReason, compact bool) {
	// keep the reasons a snasphot is being kept in a map, so that it doesn't
	// get lost when the list of snapshots is sorted
	keepReasons := make(map[restic.ID]restic.KeepReason, len(reasons))
	if len(reasons) > 0 {
		for i, sn := range list {
			id := sn.ID()
			keepReasons[*id] = reasons[i]
		}
	}

	// always sort the snapshots so that the newer ones are listed last
	sort.SliceStable(list, func(i, j int) bool {
		return list[i].Time.Before(list[j].Time)
	})

	// Determine the max widths for host and tag.
	maxHost, maxTag := 10, 6
	for _, sn := range list {
		if len(sn.Hostname) > maxHost {
			maxHost = len(sn.Hostname)
		}
		for _, tag := range sn.Tags {
			if len(tag) > maxTag {
				maxTag = len(tag)
			}
		}
	}

	tab := table.New()

	if compact {
		tab.AddColumn("ID", "{{ .ID }}")
		tab.AddColumn("Time", "{{ .Timestamp }}")
		tab.AddColumn("Host", "{{ .Hostname }}")
		tab.AddColumn("Tags  ", `{{ join .Tags "\n" }}`)
	} else {
		tab.AddColumn("ID", "{{ .ID }}")
		tab.AddColumn("Time", "{{ .Timestamp }}")
		tab.AddColumn("Host      ", "{{ .Hostname }}")
		tab.AddColumn("Tags      ", `{{ join .Tags "," }}`)
		if len(reasons) > 0 {
			tab.AddColumn("Reasons", `{{ join .Reasons "\n" }}`)
		}
		tab.AddColumn("Paths", `{{ join .Paths "\n" }}`)
	}

	type snapshot struct {
		ID        string
		Timestamp string
		Hostname  string
		Tags      []string
		Reasons   []string
		Paths     []string
	}

	var multiline bool
	for _, sn := range list {
		data := snapshot{
			ID:        sn.ID().Str(),
			Timestamp: sn.Time.Local().Format(TimeFormat),
			Hostname:  sn.Hostname,
			Tags:      sn.Tags,
			Paths:     sn.Paths,
		}

		if len(reasons) > 0 {
			id := sn.ID()
			data.Reasons = keepReasons[*id].Matches
		}

		if len(sn.Paths) > 1 && !compact {
			multiline = true
		}

		tab.AddRow(data)
	}

	tab.AddFooter(fmt.Sprintf("%d snapshots", len(list)))

	if multiline {
		// print an additional blank line between snapshots

		var last int
		tab.PrintData = func(w io.Writer, idx int, s string) error {
			var err error
			if idx == last {
				_, err = fmt.Fprintf(w, "%s\n", s)
			} else {
				_, err = fmt.Fprintf(w, "\n%s\n", s)
			}
			last = idx
			return err
		}
	}

	tab.Write(stdout)
}

// PrintSnapshotGroupHeader prints which group of the group-by option the
// following snapshots belong to.
// Prints nothing, if we did not group at all.
func PrintSnapshotGroupHeader(stdout io.Writer, groupKeyJSON string, GroupByTag bool, GroupByHost bool, GroupByPath bool) error {
	if GroupByTag || GroupByHost || GroupByPath {
		var key groupKey
		var err error

		err = json.Unmarshal([]byte(groupKeyJSON), &key)
		if err != nil {
			return err
		}

		// Info
		fmt.Fprintf(stdout, "snapshots")
		var infoStrings []string
		if GroupByTag {
			infoStrings = append(infoStrings, "tags ["+strings.Join(key.Tags, ", ")+"]")
		}
		if GroupByHost {
			infoStrings = append(infoStrings, "host ["+key.Hostname+"]")
		}
		if GroupByPath {
			infoStrings = append(infoStrings, "paths ["+strings.Join(key.Paths, ", ")+"]")
		}
		if infoStrings != nil {
			fmt.Fprintf(stdout, " for (%s)", strings.Join(infoStrings, ", "))
		}
		fmt.Fprintf(stdout, ":\n")
	}

	return nil
}

// Snapshot helps to print Snaphots as JSON with their ID included.
type Snapshot struct {
	*restic.Snapshot

	ID      *restic.ID `json:"id"`
	ShortID string     `json:"short_id"`
}

// SnapshotGroup helps to print SnaphotGroups as JSON with their GroupReasons included.
type SnapshotGroup struct {
	GroupKey  groupKey
	Snapshots []Snapshot
}

// printSnapshotsJSON writes the JSON representation of list to stdout.
func printSnapshotGroupJSON(stdout io.Writer, snGroups map[string]restic.Snapshots, grouped bool) error {

	if grouped {
		var snapshotGroups []SnapshotGroup

		for k, list := range snGroups {
			var key groupKey
			var err error
			var snapshots []Snapshot

			err = json.Unmarshal([]byte(k), &key)
			if err != nil {
				return err
			}

			for _, sn := range list {
				k := Snapshot{
					Snapshot: sn,
					ID:       sn.ID(),
					ShortID:  sn.ID().Str(),
				}
				snapshots = append(snapshots, k)
			}

			group := SnapshotGroup{
				GroupKey:  key,
				Snapshots: snapshots,
			}
			snapshotGroups = append(snapshotGroups, group)
		}

		return json.NewEncoder(stdout).Encode(snapshotGroups)
	} else {
		// Old behavior
		var snapshots []Snapshot

		for _, list := range snGroups {
			for _, sn := range list {
				k := Snapshot{
					Snapshot: sn,
					ID:       sn.ID(),
					ShortID:  sn.ID().Str(),
				}
				snapshots = append(snapshots, k)
			}
		}

		return json.NewEncoder(stdout).Encode(snapshots)
	}
}
