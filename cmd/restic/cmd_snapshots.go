package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/table"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func newSnapshotsCommand(globalOptions *GlobalOptions) *cobra.Command {
	var opts SnapshotOptions

	cmd := &cobra.Command{
		Use:   "snapshots [flags] [snapshotID ...]",
		Short: "List all snapshots",
		Long: `
The "snapshots" command lists all snapshots stored in the repository.

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
			return runSnapshots(cmd.Context(), opts, *globalOptions, args, globalOptions.term)
		},
	}

	opts.AddFlags(cmd.Flags())
	return cmd
}

// SnapshotOptions bundles all options for the snapshots command.
type SnapshotOptions struct {
	data.SnapshotFilter
	Compact bool
	Last    bool // This option should be removed in favour of Latest.
	Latest  int
	GroupBy data.SnapshotGroupByOptions
}

func (opts *SnapshotOptions) AddFlags(f *pflag.FlagSet) {
	initMultiSnapshotFilter(f, &opts.SnapshotFilter, true)
	f.BoolVarP(&opts.Compact, "compact", "c", false, "use compact output format")
	f.BoolVar(&opts.Last, "last", false, "only show the last snapshot for each host and path")
	err := f.MarkDeprecated("last", "use --latest 1")
	if err != nil {
		// MarkDeprecated only returns an error when the flag is not found
		panic(err)
	}
	f.IntVar(&opts.Latest, "latest", 0, "only show the last `n` snapshots for each host and path")
	f.VarP(&opts.GroupBy, "group-by", "g", "`group` snapshots by host, paths and/or tags, separated by comma")
}

func runSnapshots(ctx context.Context, opts SnapshotOptions, gopts GlobalOptions, args []string, term ui.Terminal) error {
	printer := ui.NewProgressPrinter(gopts.JSON, gopts.verbosity, term)
	ctx, repo, unlock, err := openWithReadLock(ctx, gopts, gopts.NoLock, printer)
	if err != nil {
		return err
	}
	defer unlock()

	var snapshots data.Snapshots
	for sn := range FindFilteredSnapshots(ctx, repo, repo, &opts.SnapshotFilter, args, printer) {
		snapshots = append(snapshots, sn)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	snapshotGroups, grouped, err := data.GroupSnapshots(snapshots, opts.GroupBy)
	if err != nil {
		return err
	}

	for k, list := range snapshotGroups {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if opts.Last {
			// This branch should be removed in the same time
			// that --last.
			list = FilterLatestSnapshots(list, 1)
		} else if opts.Latest > 0 {
			list = FilterLatestSnapshots(list, opts.Latest)
		}
		sort.Sort(sort.Reverse(list))
		snapshotGroups[k] = list
	}

	if gopts.JSON {
		err := printSnapshotGroupJSON(gopts.term.OutputWriter(), snapshotGroups, grouped)
		if err != nil {
			printer.E("error printing snapshots: %v", err)
		}
		return nil
	}

	for k, list := range snapshotGroups {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if grouped {
			err := PrintSnapshotGroupHeader(gopts.term.OutputWriter(), k)
			if err != nil {
				return err
			}
		}
		err := PrintSnapshots(gopts.term.OutputWriter(), list, nil, opts.Compact)
		if err != nil {
			return err
		}
	}

	return nil
}

// filterLastSnapshotsKey is used by FilterLastSnapshots.
type filterLastSnapshotsKey struct {
	Hostname    string
	JoinedPaths string
}

// newFilterLastSnapshotsKey initializes a filterLastSnapshotsKey from a Snapshot
func newFilterLastSnapshotsKey(sn *data.Snapshot) filterLastSnapshotsKey {
	// Shallow slice copy
	var paths = make([]string, len(sn.Paths))
	copy(paths, sn.Paths)
	sort.Strings(paths)
	return filterLastSnapshotsKey{sn.Hostname, strings.Join(paths, "|")}
}

// FilterLatestSnapshots filters a list of snapshots to only return
// the limit last entries for each hostname and path. If the snapshot
// contains multiple paths, they will be joined and treated as one
// item.
func FilterLatestSnapshots(list data.Snapshots, limit int) data.Snapshots {
	// Sort the snapshots so that the newer ones are listed first
	sort.SliceStable(list, func(i, j int) bool {
		return list[i].Time.After(list[j].Time)
	})

	var results data.Snapshots
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
func PrintSnapshots(stdout io.Writer, list data.Snapshots, reasons []data.KeepReason, compact bool) error {
	// keep the reasons a snasphot is being kept in a map, so that it doesn't
	// get lost when the list of snapshots is sorted
	keepReasons := make(map[restic.ID]data.KeepReason, len(reasons))
	if len(reasons) > 0 {
		for i, sn := range list {
			id := sn.ID()
			keepReasons[*id] = reasons[i]
		}
	}
	// check if any snapshot contains a summary
	hasSize := false
	for _, sn := range list {
		hasSize = hasSize || (sn.Summary != nil)
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
		if hasSize {
			tab.AddColumn("Size", `{{ .Size }}`)
		}
	} else {
		tab.AddColumn("ID", "{{ .ID }}")
		tab.AddColumn("Time", "{{ .Timestamp }}")
		tab.AddColumn("Host      ", "{{ .Hostname }}")
		tab.AddColumn("Tags      ", `{{ join .Tags "," }}`)
		if len(reasons) > 0 {
			tab.AddColumn("Reasons", `{{ join .Reasons "\n" }}`)
		}
		tab.AddColumn("Paths", `{{ join .Paths "\n" }}`)
		if hasSize {
			tab.AddColumn("Size", `{{ .Size }}`)
		}
	}

	type snapshot struct {
		ID        string
		Timestamp string
		Hostname  string
		Tags      []string
		Reasons   []string
		Paths     []string
		Size      string
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

		if sn.Summary != nil {
			data.Size = ui.FormatBytes(sn.Summary.TotalBytesProcessed)
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

	return tab.Write(stdout)
}

// PrintSnapshotGroupHeader prints which group of the group-by option the
// following snapshots belong to.
// Prints nothing, if we did not group at all.
func PrintSnapshotGroupHeader(stdout io.Writer, groupKeyJSON string) error {
	var key data.SnapshotGroupKey

	err := json.Unmarshal([]byte(groupKeyJSON), &key)
	if err != nil {
		return err
	}

	if key.Hostname == "" && key.Tags == nil && key.Paths == nil {
		return nil
	}

	// Info
	header := "snapshots"
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
		header += " for (" + strings.Join(infoStrings, ", ") + ")"
	}
	header += ":\n"
	_, err = stdout.Write([]byte(header))
	return err
}

// Snapshot helps to print Snapshots as JSON with their ID included.
type Snapshot struct {
	*data.Snapshot

	ID      *restic.ID `json:"id"`
	ShortID string     `json:"short_id"` // deprecated
}

// SnapshotGroup helps to print SnapshotGroups as JSON with their GroupReasons included.
type SnapshotGroup struct {
	GroupKey  data.SnapshotGroupKey `json:"group_key"`
	Snapshots []Snapshot            `json:"snapshots"`
}

// printSnapshotGroupJSON writes the JSON representation of list to stdout.
func printSnapshotGroupJSON(stdout io.Writer, snGroups map[string]data.Snapshots, grouped bool) error {
	if grouped {
		snapshotGroups := []SnapshotGroup{}

		for k, list := range snGroups {
			var key data.SnapshotGroupKey
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
