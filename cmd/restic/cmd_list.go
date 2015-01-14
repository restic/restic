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
	return "[data|trees|snapshots|keys|locks]"
}

func (cmd CmdList) Execute(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("type not specified, Usage: %s", cmd.Usage())
	}

	s, err := OpenRepo()
	if err != nil {
		return err
	}

	var (
		t    backend.Type
		each func(backend.Type, func(backend.ID, []byte, error)) error = s.Each
	)
	switch args[0] {
	case "data":
		t = backend.Data
		each = s.EachDecrypted
	case "trees":
		t = backend.Tree
		each = s.EachDecrypted
	case "snapshots":
		t = backend.Snapshot
	case "keys":
		t = backend.Key
	case "locks":
		t = backend.Lock
	default:
		return errors.New("invalid type")
	}

	return each(t, func(id backend.ID, data []byte, err error) {
		if t == backend.Data || t == backend.Tree {
			fmt.Printf("%s %s\n", id, backend.Hash(data))
		} else {
			fmt.Printf("%s\n", id)
		}
	})
}
