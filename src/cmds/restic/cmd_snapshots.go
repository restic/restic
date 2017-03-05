package main

import (
	"fmt"
	"io"
	"restic/errors"
	"sort"

	"github.com/spf13/cobra"

	"encoding/json"
	"restic"
)

var cmdSnapshots = &cobra.Command{
	Use:   "snapshots",
	Short: "list all snapshots",
	Long: `
The "snapshots" command lists all snapshots stored in the repository.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSnapshots(snapshotOptions, globalOptions, args)
	},
}

// SnapshotOptions bundle all options for the snapshots command.
type SnapshotOptions struct {
	Host  string
	Paths []string
}

var snapshotOptions SnapshotOptions

func init() {
	cmdRoot.AddCommand(cmdSnapshots)

	f := cmdSnapshots.Flags()
	f.StringVar(&snapshotOptions.Host, "host", "", "only print snapshots for this host")
	f.StringSliceVar(&snapshotOptions.Paths, "path", []string{}, "only print snapshots for this `path` (can be specified multiple times)")
}

func runSnapshots(opts SnapshotOptions, gopts GlobalOptions, args []string) error {
	if len(args) != 0 {
		return errors.Fatal("wrong number of arguments")
	}

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

	done := make(chan struct{})
	defer close(done)

	list := []*restic.Snapshot{}
	for id := range repo.List(restic.SnapshotFile, done) {
		sn, err := restic.LoadSnapshot(repo, id)
		if err != nil {
			Warnf("error loading snapshot %s: %v\n", id, err)
			continue
		}

		if restic.SamePaths(sn.Paths, opts.Paths) && (opts.Host == "" || opts.Host == sn.Hostname) {
			pos := sort.Search(len(list), func(i int) bool {
				return list[i].Time.After(sn.Time)
			})

			if pos < len(list) {
				list = append(list, nil)
				copy(list[pos+1:], list[pos:])
				list[pos] = sn
			} else {
				list = append(list, sn)
			}
		}

	}

	if gopts.JSON {
		err := printSnapshotsJSON(gopts.stdout, list)
		if err != nil {
			Warnf("error printing snapshot: %v\n", err)
		}
		return nil
	}
	printSnapshotsReadable(gopts.stdout, list)

	return nil
}

// printSnapshotsReadable prints a text table of the snapshots in list to stdout.
func printSnapshotsReadable(stdout io.Writer, list []*restic.Snapshot) {

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

// Snapshot helps to print Snaphots as JSON
type Snapshot struct {
	*restic.Snapshot

	ID *restic.ID `json:"id"`
}

// printSnapshotsJSON writes the JSON representation of list to stdout.
func printSnapshotsJSON(stdout io.Writer, list []*restic.Snapshot) error {

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
