package main

import (
	"errors"
	"fmt"
	"log"

	"github.com/fd0/khepri"
)

func commandBackup(repo *khepri.Repository, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: backup dir")
	}

	target := args[0]

	tree, err := khepri.NewTreeFromPath(repo, target)
	if err != nil {
		return err
	}

	id, err := tree.Save(repo)
	if err != nil {
		return err
	}

	sn := khepri.NewSnapshot(target)
	sn.Content = id
	snid, err := sn.Save(repo)

	if err != nil {
		log.Printf("error saving snapshopt: %v", err)
	}

	fmt.Printf("%q archived as %v\n", target, snid)

	return nil
}
