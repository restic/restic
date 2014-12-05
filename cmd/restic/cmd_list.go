package main

import (
	"errors"
	"fmt"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
)

func init() {
	commands["list"] = commandList
}

func commandList(be backend.Server, key *restic.Key, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: list [data|trees|snapshots|keys|locks]")
	}

	var (
		t    backend.Type
		each func(backend.Server, backend.Type, func(backend.ID, []byte, error)) error = backend.Each
	)
	switch args[0] {
	case "data":
		t = backend.Data
		each = key.Each
	case "trees":
		t = backend.Tree
		each = key.Each
	case "snapshots":
		t = backend.Snapshot
	case "maps":
		t = backend.Map
	case "keys":
		t = backend.Key
	case "locks":
		t = backend.Lock
	default:
		return errors.New("invalid type")
	}

	return each(be, t, func(id backend.ID, data []byte, err error) {
		if t == backend.Data || t == backend.Tree {
			fmt.Printf("%s %s\n", id, backend.Hash(data))
		} else {
			fmt.Printf("%s\n", id)
		}
	})
}
