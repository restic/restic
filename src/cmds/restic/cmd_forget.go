package main

import (
	"fmt"
	"io"
	"restic"
	"restic/backend"
	"strings"
)

// CmdForget implements the 'forget' command.
type CmdForget struct {
	Last    int `short:"l" long:"keep-last" description:"keep the last n snapshots"`
	Hourly  int `short:"H" long:"keep-hourly" description:"keep the last n hourly snapshots"`
	Daily   int `short:"d" long:"keep-daily" description:"keep the last n daily snapshots"`
	Weekly  int `short:"w" long:"keep-weekly" description:"keep the last n weekly snapshots"`
	Monthly int `short:"m" long:"keep-monthly" description:"keep the last n monthly snapshots"`
	Yearly  int `short:"y" long:"keep-yearly" description:"keep the last n yearly snapshots"`

	DryRun bool `short:"n" long:"dry-run" description:"do not delete anything, just print what would be done"`

	global *GlobalOptions
}

func init() {
	_, err := parser.AddCommand("forget",
		"removes snapshots from a repository",
		"The forget command removes snapshots according to a policy.",
		&CmdForget{global: &globalOpts})
	if err != nil {
		panic(err)
	}
}

// Usage returns usage information for 'forget'.
func (cmd CmdForget) Usage() string {
	return "[snapshot ID] ..."
}

func printSnapshots(w io.Writer, snapshots restic.Snapshots) {
	tab := NewTable()
	tab.Header = fmt.Sprintf("%-8s  %-19s  %-10s  %s", "ID", "Date", "Host", "Directory")
	tab.RowFormat = "%-8s  %-19s  %-10s  %s"

	for _, sn := range snapshots {
		if len(sn.Paths) == 0 {
			continue
		}
		id := sn.ID()
		tab.Rows = append(tab.Rows, []interface{}{id.Str(), sn.Time.Format(TimeFormat), sn.Hostname, sn.Paths[0]})

		if len(sn.Paths) > 1 {
			for _, path := range sn.Paths[1:] {
				tab.Rows = append(tab.Rows, []interface{}{"", "", "", path})
			}
		}
	}

	tab.Write(w)
}

// Execute runs the 'forget' command.
func (cmd CmdForget) Execute(args []string) error {
	repo, err := cmd.global.OpenRepository()
	if err != nil {
		return err
	}

	lock, err := lockRepoExclusive(repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	err = repo.LoadIndex()
	if err != nil {
		return err
	}

	// first, process all snapshot IDs given as arguments
	for _, s := range args {
		id, err := restic.FindSnapshot(repo, s)
		if err != nil {
			return err
		}

		if !cmd.DryRun {
			err = repo.Backend().Remove(backend.Snapshot, id.String())
			if err != nil {
				return err
			}

			cmd.global.Verbosef("removed snapshot %v\n", id.Str())
		} else {
			cmd.global.Verbosef("would removed snapshot %v\n", id.Str())
		}
	}

	policy := restic.ExpirePolicy{
		Last:    cmd.Last,
		Hourly:  cmd.Hourly,
		Daily:   cmd.Daily,
		Weekly:  cmd.Weekly,
		Monthly: cmd.Monthly,
		Yearly:  cmd.Yearly,
	}

	if policy.Empty() {
		cmd.global.Verbosef("no expire policy configured, exiting\n")
		return nil
	}

	// then, load all remaining snapshots
	snapshots, err := restic.LoadAllSnapshots(repo)
	if err != nil {
		return err
	}

	// group by hostname and dirs
	type key struct {
		Hostname string
		Dirs     string
	}

	snapshotGroups := make(map[key]restic.Snapshots)

	for _, sn := range snapshots {
		k := key{Hostname: sn.Hostname, Dirs: strings.Join(sn.Paths, ":")}
		list := snapshotGroups[k]
		list = append(list, sn)
		snapshotGroups[k] = list
	}

	for key, snapshotGroup := range snapshotGroups {
		cmd.global.Printf("snapshots for host %v, directories %v:\n\n", key.Hostname, key.Dirs)
		keep, remove := restic.ApplyPolicy(snapshotGroup, policy)

		cmd.global.Printf("keep %d snapshots:\n", len(keep))
		printSnapshots(cmd.global.stdout, keep)
		cmd.global.Printf("\n")

		cmd.global.Printf("remove %d snapshots:\n", len(remove))
		printSnapshots(cmd.global.stdout, remove)
		cmd.global.Printf("\n")

		if !cmd.DryRun {
			for _, sn := range remove {
				err = repo.Backend().Remove(backend.Snapshot, sn.ID().String())
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}
