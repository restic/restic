package main

import (
	"errors"
	"fmt"

	"restic/backend"
)

type CmdList struct {
	global *GlobalOptions
}

func init() {
	_, err := parser.AddCommand("list",
		"lists data",
		"The list command lists structures or data of a repository",
		&CmdList{global: &globalOpts})
	if err != nil {
		panic(err)
	}
}

func (cmd CmdList) Usage() string {
	return "[blobs|packs|index|snapshots|keys|locks]"
}

func (cmd CmdList) Execute(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("type not specified, Usage: %s", cmd.Usage())
	}

	repo, err := cmd.global.OpenRepository()
	if err != nil {
		return err
	}

	if !cmd.global.NoLock {
		lock, err := lockRepo(repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	var t backend.Type
	switch args[0] {
	case "blobs":
		err = repo.LoadIndex()
		if err != nil {
			return err
		}

		for _, idx := range repo.Index().All() {
			for blob := range idx.Each(nil) {
				cmd.global.Printf("%s\n", blob.ID)
			}
		}

		return nil
	case "packs":
		t = backend.Data
	case "index":
		t = backend.Index
	case "snapshots":
		t = backend.Snapshot
	case "keys":
		t = backend.Key
	case "locks":
		t = backend.Lock
	default:
		return errors.New("invalid type")
	}

	for id := range repo.List(t, nil) {
		cmd.global.Printf("%s\n", id)
	}

	return nil
}
