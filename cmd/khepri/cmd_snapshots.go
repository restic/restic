package main

import (
	"errors"
	"fmt"

	"github.com/fd0/khepri"
	"github.com/fd0/khepri/backend"
)

const TimeFormat = "02.01.2006 15:04:05 -0700"

func commandSnapshots(be backend.Server, key *khepri.Key, args []string) error {
	if len(args) != 0 {
		return errors.New("usage: snapshots")
	}

	// ch, err := khepri.NewContentHandler(be, key)
	// if err != nil {
	// 	return err
	// }

	backend.EachID(be, backend.Snapshot, func(id backend.ID) {
		// sn, err := ch.LoadSnapshot(id)
		// if err != nil {
		// 	fmt.Fprintf(os.Stderr, "error loading snapshot %s: %v\n", id, err)
		// 	return
		// }

		// fmt.Printf("snapshot %s\n    %s at %s by %s\n",
		// 	id, sn.Dir, sn.Time, sn.Username)
		fmt.Println(id)
	})

	return nil
}
