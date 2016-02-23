package main

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"restic"
	"restic/backend"
)

type Table struct {
	Header string
	Rows   [][]interface{}

	RowFormat string
}

func NewTable() Table {
	return Table{
		Rows: [][]interface{}{},
	}
}

func (t Table) Write(w io.Writer) error {
	_, err := fmt.Fprintln(w, t.Header)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, strings.Repeat("-", 70))
	if err != nil {
		return err
	}

	for _, row := range t.Rows {
		_, err = fmt.Fprintf(w, t.RowFormat+"\n", row...)
		if err != nil {
			return err
		}
	}

	return nil
}

const TimeFormat = "2006-01-02 15:04:05"

type CmdSnapshots struct {
	global *GlobalOptions
}

func init() {
	_, err := parser.AddCommand("snapshots",
		"show snapshots",
		"The snapshots command lists all snapshots stored in a repository",
		&CmdSnapshots{global: &globalOpts})
	if err != nil {
		panic(err)
	}
}

func (cmd CmdSnapshots) Usage() string {
	return ""
}

func (cmd CmdSnapshots) Execute(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("wrong number of arguments, usage: %s", cmd.Usage())
	}

	repo, err := cmd.global.OpenRepository()
	if err != nil {
		return err
	}

	lock, err := lockRepo(repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	tab := NewTable()
	tab.Header = fmt.Sprintf("%-8s  %-19s  %-10s  %s", "ID", "Date", "Source", "Directory")
	tab.RowFormat = "%-8s  %-19s  %-10s  %s"

	done := make(chan struct{})
	defer close(done)

	list := []*restic.Snapshot{}
	for id := range repo.List(backend.Snapshot, done) {
		sn, err := restic.LoadSnapshot(repo, id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error loading snapshot %s: %v\n", id, err)
			continue
		}

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

	plen, err := repo.PrefixLength(backend.Snapshot)
	if err != nil {
		return err
	}

	for _, sn := range list {
		if len(sn.Paths) == 0 {
			continue
		}
		id := sn.ID()
		tab.Rows = append(tab.Rows, []interface{}{hex.EncodeToString(id[:plen/2]), sn.Time.Format(TimeFormat), sn.Hostname, sn.Paths[0]})

		if len(sn.Paths) > 1 {
			for _, path := range sn.Paths[1:] {
				tab.Rows = append(tab.Rows, []interface{}{"", "", "", path})
			}
		}
	}

	tab.Write(os.Stdout)

	return nil
}
