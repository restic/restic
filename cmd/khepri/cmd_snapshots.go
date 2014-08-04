package main

import (
	"errors"
	"fmt"
	"log"

	"github.com/fd0/khepri"
)

const TimeFormat = "02.01.2006 15:04:05 -0700"

func commandSnapshots(repo *khepri.Repository, args []string) error {
	if len(args) != 0 {
		return errors.New("usage: snapshots")
	}

	snapshot_ids, err := repo.List(khepri.TYPE_REF)
	if err != nil {
		log.Fatalf("error loading list of snapshot ids: %v", err)
	}

	fmt.Printf("found snapshots:\n")
	for _, id := range snapshot_ids {
		snapshot, err := khepri.LoadSnapshot(repo, id)

		if err != nil {
			log.Printf("error loading snapshot %s: %v", id, err)
			continue
		}

		fmt.Printf("%s %s@%s %s %s\n",
			snapshot.Time.Format(TimeFormat),
			snapshot.Username,
			snapshot.Hostname,
			snapshot.Dir,
			id)
	}

	return nil
}
