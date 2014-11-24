package main

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/fd0/khepri"
	"github.com/fd0/khepri/backend"
)

const (
	minute = 60
	hour   = 60 * minute
	day    = 24 * hour
	week   = 7 * day
)

const TimeFormat = "2006-01-02 15:04:05"

func reltime(t time.Time) string {
	sec := uint64(time.Since(t).Seconds())

	switch {
	case sec > week:
		return t.Format(TimeFormat)
	case sec > day:
		return fmt.Sprintf("%d days ago", sec/day)
	case sec > hour:
		return fmt.Sprintf("%d hours ago", sec/hour)
	case sec > minute:
		return fmt.Sprintf("%d minutes ago", sec/minute)
	default:
		return fmt.Sprintf("%d seconds ago", sec)
	}
}

func commandSnapshots(be backend.Server, key *khepri.Key, args []string) error {
	if len(args) != 0 {
		return errors.New("usage: snapshots")
	}

	ch, err := khepri.NewContentHandler(be, key)
	if err != nil {
		return err
	}

	fmt.Printf("%-8s  %-19s  %-10s  %s\n", "ID", "Date", "Source", "Directory")
	fmt.Printf("%s\n", strings.Repeat("-", 80))

	list := []*khepri.Snapshot{}
	plen, err := backend.PrefixLength(be, backend.Snapshot)
	if err != nil {
		return err
	}

	backend.EachID(be, backend.Snapshot, func(id backend.ID) {
		sn, err := ch.LoadSnapshot(id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error loading snapshot %s: %v\n", id, err)
			return
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
	})

	for _, sn := range list {
		fmt.Printf("%-8s  %-19s  %-10s  %s\n", sn.ID()[:plen], sn.Time.Format(TimeFormat), sn.Hostname, sn.Dir)
	}

	return nil
}
