package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

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

	ctx, cancel := context.WithCancel(gopts.ctx)
	defer cancel()

	var list restic.Snapshots
	for sn := range FindFilteredSnapshots(ctx, repo, opts.Host, opts.Tags, opts.Paths, args) {
		list = append(list, sn)
	}

	if opts.Last {
		list = FilterLastSnapshots(list)
	}

	sort.Sort(sort.Reverse(list))

	if gopts.JSON {
		err := printSnapshotsJSON(gopts.stdout, list)
		if err != nil {
			Warnf("error printing snapshot: %v\n", err)
		}
		return nil
	}
	PrintSnapshots(gopts.stdout, list, nil, opts.Compact)

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

// Snapshot helps to print Snaphots as JSON with their ID included.
type Snapshot struct {
	*restic.Snapshot

	ID      *restic.ID `json:"id"`
	ShortID string     `json:"short_id"`
}

// printSnapshotsJSON writes the JSON representation of list to stdout.
func printSnapshotsJSON(stdout io.Writer, list restic.Snapshots) error {

	var snapshots []Snapshot

	for _, sn := range list {

		k := Snapshot{
			Snapshot: sn,
			ID:       sn.ID(),
			ShortID:  sn.ID().Str(),
		}
		snapshots = append(snapshots, k)
	}

	return json.NewEncoder(stdout).Encode(snapshots)
}
