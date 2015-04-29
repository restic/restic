package main

import (
	"errors"
	"fmt"

	"github.com/restic/restic/backend"
)

type CmdList struct{}

func init() {
	_, err := parser.AddCommand("list",
		"lists data",
		"The list command lists structures or data of a repository",
		&CmdList{})
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

	s, err := OpenRepo()
	if err != nil {
		return err
	}

	var t backend.Type
	switch args[0] {
	case "blobs":
		err = s.LoadIndex()
		if err != nil {
			return err
		}

		for blob := range s.Index().Each(nil) {
			fmt.Println(blob.ID)
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

	for id := range s.List(t, nil) {
		fmt.Printf("%s\n", id)
	}

	return nil
}
