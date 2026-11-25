package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"
	"github.com/restic/restic/internal/ui/table"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func newSnapshotsCommand(globalOptions *global.Options) *cobra.Command {
	var opts SnapshotOptions

	cmd := &cobra.Command{
		Use:   "snapshots [flags] [snapshotID ...]",
		Short: "List all snapshots",
		Long: `
The "snapshots" command lists all snapshots stored in the repository.

Use "--view" to choose additional columns of the text output. It takes a
comma separated list of column names and views. A view is a named collection
of columns that is expanded in place, so "--view compact,paths" shows all
columns of the compact view plus the paths. Available columns are "size",
"added", "stored", "files", "duration" and "rate", taken from the snapshot
summary when available.

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
		PreRunE: func(_ *cobra.Command, _ []string) error {
			return opts.Finalize()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			finalizeSnapshotFilter(&opts.SnapshotFilter)
			return runSnapshots(cmd.Context(), opts, *globalOptions, args, globalOptions.Term)
		},
	}

	opts.AddFlags(cmd.Flags())
	return cmd
}

// SnapshotOptions bundles all options for the snapshots command.
type SnapshotOptions struct {
	data.SnapshotFilter
	Compact bool
	View    []string
	last    bool // Deprecated in favour of Latest.
	Latest  int
	GroupBy data.SnapshotGroupByOptions

	// set by Finalize
	columns       []string
	compactLayout bool
}

func (opts *SnapshotOptions) AddFlags(f *pflag.FlagSet) {
	initMultiSnapshotFilter(f, &opts.SnapshotFilter, true)
	f.BoolVarP(&opts.Compact, "compact", "c", false, "use compact output format")
	f.StringSliceVar(&opts.View, "view", nil, "`columns` or views to show, separated by comma (views: "+snapshotViewNames()+"; columns: "+snapshotColumnNames()+")")
	f.BoolVar(&opts.last, "last", false, "only show the last snapshot for each host and path")
	err := f.MarkDeprecated("last", "use --latest 1")
	if err != nil {
		// MarkDeprecated only returns an error when the flag is not found
		panic(err)
	}
	f.IntVar(&opts.Latest, "latest", 0, "only show the last `n` snapshots for each host and path")
	f.VarP(&opts.GroupBy, "group-by", "g", "`group` snapshots by host, paths and/or tags, separated by comma")
}

func (opts *SnapshotOptions) Finalize() error {
	if opts.last && opts.Latest == 0 {
		opts.Latest = 1
	}

	if opts.Compact && len(opts.View) > 0 {
		return errors.Fatal("--compact cannot be combined with --view, use \"--view compact\" instead")
	}

	view := opts.View
	switch {
	case opts.Compact:
		view = []string{"compact"}
	case len(view) == 0:
		view = []string{"default"}
	}

	var err error
	opts.columns, opts.compactLayout, err = resolveView(view)
	return err
}

func runSnapshots(ctx context.Context, opts SnapshotOptions, gopts global.Options, args []string, term ui.Terminal) error {
	printer := progress.NewTerminalPrinter(gopts.JSON, gopts.Verbosity, term)
	ctx, repo, unlock, err := openWithReadLock(ctx, gopts, gopts.NoLock, printer)
	if err != nil {
		return err
	}
	defer unlock()

	var snapshots data.Snapshots
	err = opts.SnapshotFilter.FindAll(ctx, repo, repo, args, func(_ string, sn *data.Snapshot, err error) error {
		if err != nil {
			return err
		}
		snapshots = append(snapshots, sn)
		return nil
	})
	if err != nil {
		return err
	}

	snapshotGroups, grouped, err := data.GroupSnapshots(snapshots, opts.GroupBy)
	if err != nil {
		return err
	}

	for k, list := range snapshotGroups {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if opts.Latest > 0 {
			if grouped {
				list = filterLatestSnapshotsInGroup(list, opts.Latest)
			} else {
				list = filterLatestSnapshots(list, opts.Latest)
			}
		}
		sort.Sort(sort.Reverse(list))
		snapshotGroups[k] = list
	}

	if gopts.JSON {
		err := printSnapshotGroupJSON(gopts.Term.OutputWriter(), snapshotGroups, grouped)
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
			err := PrintSnapshotGroupHeader(gopts.Term.OutputWriter(), k)
			if err != nil {
				return err
			}
		}
		err := printSnapshotsColumns(gopts.Term.OutputWriter(), list, nil, opts.columns, opts.compactLayout)
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

// filterLatestSnapshots filters a list of snapshots to only return
// the limit last entries for each hostname and path. If the snapshot
// contains multiple paths, they will be joined and treated as one
// item.
func filterLatestSnapshots(list data.Snapshots, limit int) data.Snapshots {
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

// filterLatestSnapshotsInGroup filters a list of snapshots to only return
// the `limit` last entries. It is assumed that the snapshot list only contains
// one group of snapshots.
func filterLatestSnapshotsInGroup(list data.Snapshots, limit int) data.Snapshots {
	// Sort the snapshots so that the newer ones are listed first
	sort.SliceStable(list, func(i, j int) bool {
		return list[i].Time.After(list[j].Time)
	})

	return list[:min(limit, len(list))]
}

// snapshotColumn describes a column of the text table printed by the snapshots command.
type snapshotColumn struct {
	name    string
	header  string
	tmpl    string
	summary bool // only has a value for snapshots that contain a summary

	// the compact layout uses different widths and joins tags with newlines
	compactHeader string
	compactTmpl   string
}

// backupDuration returns how long the backup took, or zero if unknown.
func backupDuration(s *data.SnapshotSummary) time.Duration {
	return max(s.BackupEnd.Sub(s.BackupStart), 0)
}

// processedRate returns the amount of processed data per second of backup
// time, expressed in MiB/s (matching the IEC units used elsewhere).
func processedRate(s *data.SnapshotSummary) (float64, bool) {
	d := backupDuration(s)
	if d <= 0 {
		return 0, false
	}
	return float64(s.TotalBytesProcessed) / (1024 * 1024) / d.Seconds(), true
}

var snapshotColumns = []snapshotColumn{
	{name: "id", header: "ID", tmpl: "{{ .ID }}"},
	{name: "time", header: "Time", tmpl: "{{ .Timestamp }}"},
	{name: "host", header: "Host      ", tmpl: "{{ .Hostname }}", compactHeader: "Host"},
	{name: "tags", header: "Tags      ", tmpl: `{{ join .Tags "," }}`, compactHeader: "Tags  ", compactTmpl: `{{ join .Tags "\n" }}`},
	{name: "paths", header: "Paths", tmpl: `{{ join .Paths "\n" }}`},
	{name: "size", header: "Size", tmpl: "{{ .Size }}", summary: true},
	{name: "added", header: "Added", tmpl: "{{ .Added }}", summary: true},
	{name: "stored", header: "Stored", tmpl: "{{ .Stored }}", summary: true},
	{name: "files", header: "Files", tmpl: "{{ .Files }}", summary: true},
	{name: "duration", header: "Duration", tmpl: "{{ .Duration }}", summary: true},
	{name: "rate", header: "Rate", tmpl: "{{ .Rate }}", summary: true},
}

// snapshotViews are named collections of columns.
var snapshotViews = map[string][]string{
	"default": {"id", "time", "host", "tags", "paths"},
	"compact": {"id", "time", "host", "tags", "size", "stored"},
	"growth":  {"id", "time", "host", "size", "added", "stored", "files"},
}

func snapshotColumnNames() string {
	names := make([]string, 0, len(snapshotColumns))
	for _, c := range snapshotColumns {
		names = append(names, c.name)
	}
	return strings.Join(names, ", ")
}

func findSnapshotColumn(name string) (snapshotColumn, bool) {
	for _, c := range snapshotColumns {
		if c.name == name {
			return c, true
		}
	}
	return snapshotColumn{}, false
}

// resolveView expands a list of view and column names into the ordered list of
// columns to print, without duplicates. Names are case-insensitive.
func resolveView(names []string) (columns []string, compact bool, err error) {
	seen := make(map[string]struct{})
	add := func(column string) {
		if _, ok := seen[column]; !ok {
			seen[column] = struct{}{}
			columns = append(columns, column)
		}
	}

	for _, name := range names {
		name = strings.ToLower(strings.TrimSpace(name))
		if view, ok := snapshotViews[name]; ok {
			compact = compact || name == "compact"
			for _, column := range view {
				add(column)
			}
		} else if _, ok := findSnapshotColumn(name); ok {
			add(name)
		} else {
			return nil, false, errors.Fatalf("unknown view or column %q, valid views are: %s; valid columns are: %s",
				name, snapshotViewNames(), snapshotColumnNames())
		}
	}
	return columns, compact, nil
}

func snapshotViewNames() string {
	names := make([]string, 0, len(snapshotViews))
	for name := range snapshotViews {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// PrintSnapshots prints a text table of the snapshots in list to stdout.
func PrintSnapshots(stdout io.Writer, list data.Snapshots, reasons []data.KeepReason, compact bool) error {
	view := "default"
	if compact {
		view = "compact"
	}
	return printSnapshotsColumns(stdout, list, reasons, snapshotViews[view], compact)
}

// printSnapshotsColumns prints a text table of the snapshots with the given columns.
// Summary columns are omitted if no snapshot has a summary.
func printSnapshotsColumns(stdout io.Writer, list data.Snapshots, reasons []data.KeepReason, columns []string, compact bool) error {
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

	var hasPaths bool
	for _, name := range columns {
		col, _ := findSnapshotColumn(name)
		if col.summary && !hasSize {
			continue
		}
		if name == "paths" {
			hasPaths = true
			if len(reasons) > 0 && !compact {
				tab.AddColumn("Reasons", `{{ join .Reasons "\n" }}`)
			}
		}
		header, tmpl := col.header, col.tmpl
		if compact {
			if col.compactHeader != "" {
				header = col.compactHeader
			}
			if col.compactTmpl != "" {
				tmpl = col.compactTmpl
			}
		}
		tab.AddColumn(header, tmpl)
	}

	type snapshot struct {
		ID        string
		Timestamp string
		Hostname  string
		Tags      []string
		Reasons   []string
		Paths     []string
		Size      string
		Added     string
		Stored    string
		Files     string
		Duration  string
		Rate      string
	}

	var multiline bool
	for _, sn := range list {
		data := snapshot{
			ID:        sn.ID().Str(),
			Timestamp: sn.Time.Local().Format(global.TimeFormat),
			Hostname:  sn.Hostname,
			Tags:      sn.Tags,
			Paths:     sn.Paths,
		}

		if len(reasons) > 0 {
			id := sn.ID()
			data.Reasons = keepReasons[*id].Matches
		}

		if hasPaths && len(sn.Paths) > 1 {
			multiline = true
		}

		if sn.Summary != nil {
			data.Size = ui.FormatBytes(sn.Summary.TotalBytesProcessed)
			data.Added = ui.FormatBytes(sn.Summary.DataAdded)
			data.Stored = ui.FormatBytes(sn.Summary.DataAddedPacked)
			data.Files = fmt.Sprintf("%d", sn.Summary.TotalFilesProcessed)
			data.Duration = ui.FormatDuration(backupDuration(sn.Summary))
			if rate, ok := processedRate(sn.Summary); ok {
				data.Rate = fmt.Sprintf("%.2f MiB/s", rate)
			}
		}

		tab.AddRow(data)
	}

	// Add timezone information to prevent confusion:
	// Each snapshot can be registered in different timezones,
	// but we display them all in local timezone on this output.
	footer := fmt.Sprintf("%d snapshots", len(list))
	zoneName := time.Now().Local().Location().String()
	if zoneName == "Local" {
		zoneName = "local time"
	}
	tab.AddFooter(fmt.Sprintf("Timestamps shown in %s\n%s", zoneName, footer))

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
