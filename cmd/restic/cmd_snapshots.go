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
	Use:   "snapshots [flags] [snapshotID ...]",
	Short: "List all snapshots",
	Long: `
The "snapshots" command lists all snapshots stored in the repository.

EXIT STATUS
===========

Exit status is 0 if the command was successful, and non-zero if there was any error.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSnapshots(cmd.Context(), snapshotOptions, globalOptions, args)
	},
}

// SnapshotOptions bundles all options for the snapshots command.
type SnapshotOptions struct {
	restic.SnapshotFilter
	Compact bool
	Last    bool // This option should be removed in favour of Latest.
	Latest  int
	GroupBy restic.SnapshotGroupByOptions
}

var snapshotOptions SnapshotOptions

func init() {
	cmdRoot.AddCommand(cmdSnapshots)

	f := cmdSnapshots.Flags()
	initMultiSnapshotFilter(f, &snapshotOptions.SnapshotFilter, true)
	f.BoolVarP(&snapshotOptions.Compact, "compact", "c", false, "use compact output format")
	f.BoolVar(&snapshotOptions.Last, "last", false, "only show the last snapshot for each host and path")
	err := f.MarkDeprecated("last", "use --latest 1")
	if err != nil {
		// MarkDeprecated only returns an error when the flag is not found
		panic(err)
	}
	f.IntVar(&snapshotOptions.Latest, "latest", 0, "only show the last `n` snapshots for each host and path")
	f.VarP(&snapshotOptions.GroupBy, "group-by", "g", "`group` snapshots by host, paths and/or tags, separated by comma")
}

func runSnapshots(ctx context.Context, opts SnapshotOptions, gopts GlobalOptions, args []string) error {
	repo, err := OpenRepository(ctx, gopts)
	if err != nil {
		return err
	}

	if !gopts.NoLock {
		var lock *restic.Lock
		lock, ctx, err = lockRepo(ctx, repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	var snapshots restic.Snapshots
	for sn := range FindFilteredSnapshots(ctx, repo.Backend(), repo, &opts.SnapshotFilter, args) {
		snapshots = append(snapshots, sn)
	}
	snapshotGroups, grouped, err := restic.GroupSnapshots(snapshots, opts.GroupBy)
	if err != nil {
		return err
	}

	for k, list := range snapshotGroups {
		if opts.Last {
			// This branch should be removed in the same time
			// that --last.
			list = FilterLastestSnapshots(list, 1)
		} else if opts.Latest > 0 {
			list = FilterLastestSnapshots(list, opts.Latest)
		}
		sort.Sort(sort.Reverse(list))
		snapshotGroups[k] = list
	}

	if gopts.JSON {
		err := printSnapshotGroupJSON(gopts.stdout, snapshotGroups, grouped)
		if err != nil {
			Warnf("error printing snapshots: %v\n", err)
		}
		return nil
	}

	for k, list := range snapshotGroups {
		if grouped {
			err := PrintSnapshotGroupHeader(gopts.stdout, k)
			if err != nil {
				Warnf("error printing snapshots: %v\n", err)
				return nil
			}
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

// FilterLastestSnapshots filters a list of snapshots to only return
// the limit last entries for each hostname and path. If the snapshot
// contains multiple paths, they will be joined and treated as one
// item.
func FilterLastestSnapshots(list restic.Snapshots, limit int) restic.Snapshots {
	// Sort the snapshots so that the newer ones are listed first
	sort.SliceStable(list, func(i, j int) bool {
		return list[i].Time.After(list[j].Time)
	})

	var results restic.Snapshots
	seen := make(map[filterLastSnapshotsKey]int)
	for _, sn := range list {
		key := newFilterLastSnapshotsKey(sn)
		if seen[key] < limit {
			seen[key]++
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

	err := tab.Write(stdout)
	if err != nil {
		Warnf("error printing: %v\n", err)
	}
}

// PrintSnapshotGroupHeader prints which group of the group-by option the
// following snapshots belong to.
// Prints nothing, if we did not group at all.
func PrintSnapshotGroupHeader(stdout io.Writer, groupKeyJSON string) error {
	var key restic.SnapshotGroupKey

	err := json.Unmarshal([]byte(groupKeyJSON), &key)
	if err != nil {
		return err
	}

	if key.Hostname == "" && key.Tags == nil && key.Paths == nil {
		return nil
	}

	// Info
	fmt.Fprintf(stdout, "snapshots")
	var infoStrings []string
	if key.Hostname != "" {
		infoStrings = append(infoStrings, "host ["+key.Hostname+"]")
	}
	if key.Tags != nil {
		infoStrings = append(infoStrings, "tags ["+strings.Join(key.Tags, ", ")+"]")
	}
	if key.Paths != nil {
		infoStrings = append(infoStrings, "paths ["+strings.Join(key.Paths, ", ")+"]")
	}
	if infoStrings != nil {
		fmt.Fprintf(stdout, " for (%s)", strings.Join(infoStrings, ", "))
	}
	fmt.Fprintf(stdout, ":\n")

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
	GroupKey  restic.SnapshotGroupKey `json:"group_key"`
	Snapshots []Snapshot              `json:"snapshots"`
}

// printSnapshotsJSON writes the JSON representation of list to stdout.
func printSnapshotGroupJSON(stdout io.Writer, snGroups map[string]restic.Snapshots, grouped bool) error {
	if grouped {
		snapshotGroups := []SnapshotGroup{}

		for k, list := range snGroups {
			var key restic.SnapshotGroupKey
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
	}

	// Old behavior
	snapshots := []Snapshot{}

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
