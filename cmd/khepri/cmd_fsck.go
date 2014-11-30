package main

import (
	"errors"
	"fmt"

	"github.com/fd0/khepri"
	"github.com/fd0/khepri/backend"
)

func init() {
	commands["fsck"] = commandFsck
}

func fsckFile(ch *khepri.ContentHandler, IDs []backend.ID) error {
	for _, id := range IDs {
		debug("checking data blob %v\n", id)

		// load content
		_, err := ch.Load(backend.Data, id)
		if err != nil {
			return err
		}
	}

	return nil
}

func fsckTree(ch *khepri.ContentHandler, id backend.ID) error {
	debug("checking tree %v\n", id)

	tree, err := khepri.LoadTree(ch, id)
	if err != nil {
		return err
	}

	for i, node := range tree {
		if node.Name == "" {
			return fmt.Errorf("node %v of tree %v has no name", i, id)
		}

		if node.Type == "" {
			return fmt.Errorf("node %q of tree %v has no type", node.Name, id)
		}

		switch node.Type {
		case "file":
			if node.Content == nil {
				return fmt.Errorf("file node %q of tree %v has no content", node.Name, id)
			}

			err := fsckFile(ch, node.Content)
			if err != nil {
				return err
			}
		case "dir":
			if node.Subtree == nil {
				return fmt.Errorf("dir node %q of tree %v has no subtree", node.Name, id)
			}

			err := fsckTree(ch, node.Subtree)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func fsck_snapshot(be backend.Server, key *khepri.Key, id backend.ID) error {
	debug("checking snapshot %v\n", id)

	ch, err := khepri.NewContentHandler(be, key)
	if err != nil {
		return err
	}

	sn, err := ch.LoadSnapshot(id)
	if err != nil {
		return err
	}

	if sn.Content == nil {
		return fmt.Errorf("snapshot %v has no content", sn.ID)
	}

	if sn.Map == nil {
		return fmt.Errorf("snapshot %v has no map", sn.ID)
	}

	return fsckTree(ch, sn.Content)
}

func commandFsck(be backend.Server, key *khepri.Key, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: fsck [all|snapshot-id]")
	}

	if len(args) == 1 && args[0] != "all" {
		snapshotID, err := backend.FindSnapshot(be, args[0])
		if err != nil {
			return fmt.Errorf("invalid id %q: %v", args[1], err)
		}

		return fsck_snapshot(be, key, snapshotID)
	}

	list, err := be.List(backend.Snapshot)
	if err != nil {
		return err
	}

	for _, snapshotID := range list {
		err := fsck_snapshot(be, key, snapshotID)

		if err != nil {
			return err
		}
	}

	return nil
}
