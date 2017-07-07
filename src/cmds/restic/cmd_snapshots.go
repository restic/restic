package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/spf13/cobra"

	"restic"
)

var cmdSnapshots = &cobra.Command{
	Use:   "snapshots [snapshotID ...]",
	Short: "list all snapshots",
	Long: `
The "snapshots" command lists all snapshots stored in the repository.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSnapshots(snapshotOptions, globalOptions, args)
	},
}

// SnapshotOptions bundles all options for the snapshots command.
type SnapshotOptions struct {
	Host  string
	Tags  []string
	Paths []string
}

var snapshotOptions SnapshotOptions

func init() {
	cmdRoot.AddCommand(cmdSnapshots)

	f := cmdSnapshots.Flags()
	f.StringVarP(&snapshotOptions.Host, "host", "H", "", "only consider snapshots for this `host`")
	f.StringArrayVar(&snapshotOptions.Tags, "tag", nil, "only consider snapshots which include this `tag` (can be specified multiple times)")
	f.StringArrayVar(&snapshotOptions.Paths, "path", nil, "only consider snapshots for this `path` (can be specified multiple times)")
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
	sort.Sort(sort.Reverse(list))

	if gopts.JSON {
		err := printSnapshotsJSON(gopts.stdout, list)
		if err != nil {
			Warnf("error printing snapshot: %v\n", err)
		}
		return nil
	}
	PrintSnapshots(gopts.stdout, list)

	return nil
}

// PrintSnapshots prints a text table of the snapshots in list to stdout.
func PrintSnapshots(stdout io.Writer, list restic.Snapshots) {

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

	tab := NewTable()
	tab.Header = fmt.Sprintf("%-8s  %-19s  %-*s  %-*s  %-3s %s", "ID", "Date", -maxHost, "Host", -maxTag, "Tags", "", "Directory")
	tab.RowFormat = fmt.Sprintf("%%-8s  %%-19s  %%%ds  %%%ds  %%-3s %%s", -maxHost, -maxTag)

	for _, sn := range list {
		if len(sn.Paths) == 0 {
			continue
		}

		firstTag := ""
		if len(sn.Tags) > 0 {
			firstTag = sn.Tags[0]
		}

		rows := len(sn.Paths)
		if rows < len(sn.Tags) {
			rows = len(sn.Tags)
		}

		treeElement := "   "
		if rows != 1 {
			treeElement = "┌──"
		}

		tab.Rows = append(tab.Rows, []interface{}{sn.ID().Str(), sn.Time.Format(TimeFormat), sn.Hostname, firstTag, treeElement, sn.Paths[0]})

		if len(sn.Tags) > rows {
			rows = len(sn.Tags)
		}

		for i := 1; i < rows; i++ {
			path := ""
			if len(sn.Paths) > i {
				path = sn.Paths[i]
			}

			tag := ""
			if len(sn.Tags) > i {
				tag = sn.Tags[i]
			}

			treeElement := "│"
			if i == (rows - 1) {
				treeElement = "└──"
			}

			tab.Rows = append(tab.Rows, []interface{}{"", "", "", tag, treeElement, path})
		}
	}

	tab.Write(stdout)
}

// Snapshot helps to print Snaphots as JSON with their ID included.
type Snapshot struct {
	*restic.Snapshot

	ID *restic.ID `json:"id"`
}

// printSnapshotsJSON writes the JSON representation of list to stdout.
func printSnapshotsJSON(stdout io.Writer, list restic.Snapshots) error {

	var snapshots []Snapshot

	for _, sn := range list {

		k := Snapshot{
			Snapshot: sn,
			ID:       sn.ID(),
		}
		snapshots = append(snapshots, k)
	}

	return json.NewEncoder(stdout).Encode(snapshots)
}
