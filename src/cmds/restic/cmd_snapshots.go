package main

import (
	"fmt"
	"os"
	"restic/errors"
	"sort"

	"github.com/spf13/cobra"

	"restic"
)

var cmdSnapshots = &cobra.Command{
	Use:   "snapshots",
	Short: "list all snapshots",
	Long: `
The "snapshots" command lists all snapshots stored in a repository.
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
		return errors.Fatalf("wrong number of arguments")
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

	tab := NewTable()
	tab.Header = fmt.Sprintf("%-8s  %-19s  %-10s  %-10s  %s", "ID", "Date", "Host", "Tags", "Directory")
	tab.RowFormat = "%-8s  %-19s  %-10s  %-10s  %s"

	done := make(chan struct{})
	defer close(done)

	list := []*restic.Snapshot{}
	for id := range repo.List(restic.SnapshotFile, done) {
		sn, err := restic.LoadSnapshot(repo, id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error loading snapshot %s: %v\n", id, err)
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

	for _, sn := range list {
		if len(sn.Paths) == 0 {
			continue
		}

		firstTag := ""
		if len(sn.Tags) > 0 {
			firstTag = sn.Tags[0]
		}

		tab.Rows = append(tab.Rows, []interface{}{sn.ID().Str(), sn.Time.Format(TimeFormat), sn.Hostname, firstTag, sn.Paths[0]})

		rows := len(sn.Paths)
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

			tab.Rows = append(tab.Rows, []interface{}{"", "", "", tag, path})
		}
	}

	tab.Write(os.Stdout)

	return nil
}
