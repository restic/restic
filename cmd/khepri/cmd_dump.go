package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/fd0/khepri"
)

func dump_tree(repo *khepri.Repository, id khepri.ID) error {
	tree, err := khepri.NewTreeFromRepo(repo, id)
	if err != nil {
		return err
	}

	buf, err := json.MarshalIndent(tree, "", "  ")
	if err != nil {
		return err
	}

	fmt.Printf("tree %s\n%s\n", id, buf)

	for _, node := range tree.Nodes {
		if node.Type == "dir" {
			err = dump_tree(repo, node.Subtree)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
			}
		}
	}

	return nil
}

func dump_snapshot(repo *khepri.Repository, id khepri.ID) error {
	sn, err := khepri.LoadSnapshot(repo, id)
	if err != nil {
		log.Fatalf("error loading snapshot %s", id)
	}

	buf, err := json.MarshalIndent(sn, "", "  ")
	if err != nil {
		return err
	}

	fmt.Printf("%s\n%s\n", sn, buf)

	return dump_tree(repo, sn.Content)
}

func dump_file(repo *khepri.Repository, id khepri.ID) error {
	rd, err := repo.Get(khepri.TYPE_BLOB, id)
	if err != nil {
		return err
	}

	io.Copy(os.Stdout, rd)

	return nil
}

func commandDump(repo *khepri.Repository, args []string) error {
	if len(args) != 2 {
		return errors.New("usage: dump [snapshot|tree|file] ID")
	}

	tpe := args[0]

	id, err := khepri.ParseID(args[1])
	if err != nil {
		errx(1, "invalid id %q: %v", args[0], err)
	}

	switch tpe {
	case "snapshot":
		return dump_snapshot(repo, id)
	case "tree":
		return dump_tree(repo, id)
	case "file":
		return dump_file(repo, id)
	default:
		return fmt.Errorf("invalid type %q", tpe)
	}

	return nil
}
