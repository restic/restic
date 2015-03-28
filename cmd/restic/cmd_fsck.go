package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/debug"
)

type CmdFsck struct {
	CheckData      bool   `          long:"check-data"      description:"Read data blobs" default:"false"`
	Snapshot       string `short:"s" long:"snapshot"        description:"Only check this snapshot"`
	Orphaned       bool   `short:"o" long:"orphaned"        description:"Check for orphaned blobs"`
	RemoveOrphaned bool   `short:"r" long:"remove-orphaned" description:"Remove orphaned blobs (implies -o)"`

	// lists checking for orphaned blobs
	o_data  *backend.IDSet
	o_trees *backend.IDSet
}

func init() {
	_, err := parser.AddCommand("fsck",
		"check the repository",
		"The fsck command check the integrity and consistency of the repository",
		&CmdFsck{})
	if err != nil {
		panic(err)
	}
}

func fsckFile(opts CmdFsck, s restic.Server, m *restic.Map, IDs []backend.ID) (uint64, error) {
	debug.Log("restic.fsckFile", "checking file %v", IDs)
	var bytes uint64

	for _, id := range IDs {
		debug.Log("restic.fsck", "  checking data blob %v\n", id)

		// test if blob is in map
		blob, err := m.FindID(id)
		if err != nil {
			return 0, fmt.Errorf("storage ID for data blob %v not found", id)
		}

		bytes += blob.Size
		debug.Log("restic.fsck", "  data blob found: %v\n", blob)

		if opts.CheckData {
			// load content
			_, err := s.Load(backend.Data, blob)
			if err != nil {
				return 0, err
			}
		} else {
			// test if data blob is there
			ok, err := s.Test(backend.Data, blob.Storage.String())
			if err != nil {
				return 0, err
			}

			if !ok {
				return 0, fmt.Errorf("data blob %v not found", id)
			}
		}

		// if orphan check is active, record storage id
		if opts.o_data != nil {
			opts.o_data.Insert(blob.Storage)
		}
	}

	return bytes, nil
}

func fsckTree(opts CmdFsck, s restic.Server, blob restic.Blob) error {
	debug.Log("restic.fsckTree", "checking tree %v", blob)

	tree, err := restic.LoadTree(s, blob.Storage)
	if err != nil {
		return err
	}

	// if orphan check is active, record storage id
	if opts.o_trees != nil {
		// add ID to list
		opts.o_trees.Insert(blob.Storage)
	}

	var firstErr error

	seenIDs := backend.NewIDSet()

	for i, node := range tree.Nodes {
		if node.Name == "" {
			return fmt.Errorf("node %v of tree %v has no name", i, blob.ID)
		}

		if node.Type == "" {
			return fmt.Errorf("node %q of tree %v has no type", node.Name, blob.ID)
		}

		switch node.Type {
		case "file":
			if node.Content == nil {
				debug.Log("restic.fsckTree", "file node %q of tree %v has no content: %v", node.Name, blob.ID, node)
				return fmt.Errorf("file node %q of tree %v has no content: %v", node.Name, blob.ID, node)
			}

			if node.Content == nil && node.Error == "" {
				debug.Log("restic.fsckTree", "file node %q of tree %v has no content", node.Name, blob.ID)
				return fmt.Errorf("file node %q of tree %v has no content", node.Name, blob.ID)
			}

			// record ids
			for _, id := range node.Content {
				seenIDs.Insert(id)
			}

			debug.Log("restic.fsckTree", "check file %v (%v)", node.Name, blob.ID.Str())
			bytes, err := fsckFile(opts, s, tree.Map, node.Content)
			if err != nil {
				return err
			}

			if bytes != node.Size {
				debug.Log("restic.fsckTree", "file node %q of tree %v has size %d, but only %d bytes could be found", node.Name, blob, node.Size, bytes)
				return fmt.Errorf("file node %q of tree %v has size %d, but only %d bytes could be found", node.Name, blob, node.Size, bytes)
			}
		case "dir":
			if node.Subtree == nil {
				return fmt.Errorf("dir node %q of tree %v (storage id %v) has no subtree", node.Name, blob.ID, blob.Storage)
			}

			// lookup blob
			subtreeBlob, err := tree.Map.FindID(node.Subtree)
			if err != nil {
				firstErr = err
				fmt.Fprintf(os.Stderr, "%v\n", err)
			}

			// record id
			seenIDs.Insert(node.Subtree)

			err = fsckTree(opts, s, subtreeBlob)
			if err != nil {
				firstErr = err
				fmt.Fprintf(os.Stderr, "%v\n", err)
			}
		}
	}

	// check map for unused ids
	for _, id := range tree.Map.IDs() {
		if seenIDs.Find(id) != nil {
			return fmt.Errorf("tree %v: map contains unused ID %v", blob.ID, id)
		}
	}

	return firstErr
}

func fsck_snapshot(opts CmdFsck, s restic.Server, id backend.ID) error {
	debug.Log("restic.fsck", "checking snapshot %v\n", id)

	sn, err := restic.LoadSnapshot(s, id)
	if err != nil {
		return fmt.Errorf("loading snapshot %v failed: %v", id, err)
	}

	if !sn.Tree.Valid() {
		return fmt.Errorf("snapshot %s has invalid tree %v", sn.ID(), sn.Tree)
	}

	err = fsckTree(opts, s, sn.Tree)
	if err != nil {
		debug.Log("restic.fsck", "  checking tree %v for snapshot %v\n", sn.Tree, id)
		fmt.Fprintf(os.Stderr, "snapshot %v:\n  error for tree %v:\n    %v\n", id, sn.Tree, err)
	}

	return err
}

func (cmd CmdFsck) Usage() string {
	return "[fsck-options]"
}

func (cmd CmdFsck) Execute(args []string) error {
	if len(args) != 0 {
		return errors.New("fsck has no arguments")
	}

	if cmd.RemoveOrphaned && !cmd.Orphaned {
		cmd.Orphaned = true
	}

	s, err := OpenRepo()
	if err != nil {
		return err
	}

	if cmd.Snapshot != "" {
		name, err := s.FindSnapshot(cmd.Snapshot)
		if err != nil {
			return fmt.Errorf("invalid id %q: %v", cmd.Snapshot, err)
		}

		id, err := backend.ParseID(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid snapshot id %v\n", name)
		}

		err = fsck_snapshot(cmd, s, id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "check for snapshot %v failed\n", id)
		}

		return err
	}

	if cmd.Orphaned {
		cmd.o_data = backend.NewIDSet()
		cmd.o_trees = backend.NewIDSet()
	}

	done := make(chan struct{})
	defer close(done)

	var firstErr error
	for name := range s.List(backend.Snapshot, done) {
		id, err := backend.ParseID(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid snapshot id %v\n", name)
			continue
		}

		err = fsck_snapshot(cmd, s, id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "check for snapshot %v failed\n", id)
			firstErr = err
		}
	}

	if !cmd.Orphaned {
		return firstErr
	}

	debug.Log("restic.fsck", "starting orphaned check\n")

	l := []struct {
		desc string
		tpe  backend.Type
		set  *backend.IDSet
	}{
		{"data blob", backend.Data, cmd.o_data},
		{"tree", backend.Tree, cmd.o_trees},
	}

	for _, d := range l {
		debug.Log("restic.fsck", "checking for orphaned %v\n", d.desc)

		done := make(chan struct{})

		for name := range s.List(d.tpe, done) {
			id, err := backend.ParseID(name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "invalid id for %v: %v\n", d.tpe, name)
				continue
			}

			err = d.set.Find(id)
			if err != nil {
				if !cmd.RemoveOrphaned {
					fmt.Printf("orphaned %v %v\n", d.desc, id)
					continue
				}

				fmt.Printf("removing orphaned %v %v\n", d.desc, id)
				err := s.Remove(d.tpe, name)
				if err != nil {
					return err
				}
			}
		}
	}

	return firstErr
}
