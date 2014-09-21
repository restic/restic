package main

import (
	"errors"
	"log"

	"github.com/fd0/khepri"
)

func commandRestore(repo *khepri.Repository, args []string) error {
	if len(args) != 2 {
		return errors.New("usage: restore ID dir")
	}

	id, err := khepri.ParseID(args[0])
	if err != nil {
		errx(1, "invalid id %q: %v", args[0], err)
	}

	target := args[1]

	sn, err := khepri.LoadSnapshot(repo, id)
	if err != nil {
		log.Fatalf("error loading snapshot %s: %v", id, err)
	}

	err = sn.RestoreAt(target)
	if err != nil {
		log.Fatalf("error restoring snapshot %s: %v", id, err)
	}

	log.Printf("%q restored to %q\n", id, target)

	return nil
}
