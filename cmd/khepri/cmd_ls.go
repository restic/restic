package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fd0/khepri"
	"github.com/fd0/khepri/backend"
)

func print_node(prefix string, n *khepri.Node) string {
	switch n.Type {
	case "file":
		return fmt.Sprintf("%s %5d %5d %6d %s %s",
			n.Mode, n.UID, n.GID, n.Size, n.ModTime, filepath.Join(prefix, n.Name))
	case "dir":
		return fmt.Sprintf("%s %5d %5d %6d %s %s",
			n.Mode|os.ModeDir, n.UID, n.GID, n.Size, n.ModTime, filepath.Join(prefix, n.Name))
	case "symlink":
		return fmt.Sprintf("%s %5d %5d %6d %s %s -> %s",
			n.Mode|os.ModeSymlink, n.UID, n.GID, n.Size, n.ModTime, filepath.Join(prefix, n.Name), n.LinkTarget)
	default:
		return fmt.Sprintf("<Node(%s) %s>", n.Type, n.Name)
	}
}

func print_tree(prefix string, ch *khepri.ContentHandler, id backend.ID) error {
	tree := &khepri.Tree{}

	err := ch.LoadJSON(backend.Tree, id, tree)
	if err != nil {
		return err
	}

	for _, entry := range *tree {
		fmt.Println(print_node(prefix, entry))

		if entry.Type == "dir" && entry.Subtree != nil {
			err = print_tree(filepath.Join(prefix, entry.Name), ch, entry.Subtree)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func commandLs(be backend.Server, key *khepri.Key, args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return errors.New("usage: ls SNAPSHOT_ID [dir]")
	}

	id, err := backend.ParseID(args[0])
	if err != nil {
		return err
	}

	ch, err := khepri.NewContentHandler(be, key)
	if err != nil {
		return err
	}

	sn, err := ch.LoadSnapshot(id)
	if err != nil {
		return err
	}

	fmt.Printf("snapshot of %s at %s:\n", sn.Dir, sn.Time)

	return print_tree("", ch, sn.Content)
}
