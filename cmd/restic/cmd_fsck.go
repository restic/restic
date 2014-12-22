package main

import (
	"errors"
	"fmt"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
)

type CmdFsck struct {
	CheckData      bool   `          long:"check-data"      description:"Read data blobs" default:"false"`
	Snapshot       string `short:"s" long:"snapshot"        description:"Only check this snapshot"`
	Orphaned       bool   `short:"o" long:"orphaned"        description:"Check for orphaned blobs"`
	RemoveOrphaned bool   `short:"x" long:"remove-orphaned" description:"Remove orphaned blobs (implies -o)"`

	// lists checking for orphaned blobs
	o_data  *restic.BlobList
	o_trees *restic.BlobList
	o_maps  *restic.BlobList
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

func fsckFile(opts CmdFsck, ch *restic.ContentHandler, IDs []backend.ID) error {
	for _, id := range IDs {
		debug("checking data blob %v\n", id)

		if opts.CheckData {
			// load content
			_, err := ch.Load(backend.Data, id)
			if err != nil {
				return err
			}
		} else {
			// test if data blob is there
			ok, err := ch.Test(backend.Data, id)
			if err != nil {
				return err
			}

			if !ok {
				return fmt.Errorf("data blob %v not found", id)
			}
		}

		// if orphan check is active, record storage id
		if opts.o_data != nil {
			// lookup storage ID
			sid, err := ch.Lookup(id)
			if err != nil {
				return err
			}

			// add ID to list
			opts.o_data.Insert(restic.Blob{ID: sid})
		}
	}

	return nil
}

func fsckTree(opts CmdFsck, ch *restic.ContentHandler, id backend.ID) error {
	debug("checking tree %v\n", id)

	tree, err := restic.LoadTree(ch, id)
	if err != nil {
		return err
	}

	// if orphan check is active, record storage id
	if opts.o_trees != nil {
		// lookup storage ID
		sid, err := ch.Lookup(id)
		if err != nil {
			return err
		}

		// add ID to list
		opts.o_trees.Insert(restic.Blob{ID: sid})
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

			err := fsckFile(opts, ch, node.Content)
			if err != nil {
				return err
			}
		case "dir":
			if node.Subtree == nil {
				return fmt.Errorf("dir node %q of tree %v has no subtree", node.Name, id)
			}

			err := fsckTree(opts, ch, node.Subtree)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func fsck_snapshot(opts CmdFsck, s restic.Server, id backend.ID) error {
	debug("checking snapshot %v\n", id)

	ch, err := restic.NewContentHandler(s)
	if err != nil {
		return err
	}

	sn, err := ch.LoadSnapshot(id)
	if err != nil {
		return err
	}

	if sn.Tree == nil {
		return fmt.Errorf("snapshot %v has no content", sn.ID)
	}

	if sn.Map == nil {
		return fmt.Errorf("snapshot %v has no map", sn.ID)
	}

	// if orphan check is active, record storage id for map
	if opts.o_maps != nil {
		opts.o_maps.Insert(restic.Blob{ID: sn.Map})
	}

	return fsckTree(opts, ch, sn.Tree)
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
		snapshotID, err := s.FindSnapshot(cmd.Snapshot)
		if err != nil {
			return fmt.Errorf("invalid id %q: %v", cmd.Snapshot, err)
		}

		return fsck_snapshot(cmd, s, snapshotID)
	}

	if cmd.Orphaned {
		cmd.o_data = restic.NewBlobList()
		cmd.o_trees = restic.NewBlobList()
		cmd.o_maps = restic.NewBlobList()
	}

	list, err := s.List(backend.Snapshot)
	debug("checking %d snapshots\n", len(list))
	if err != nil {
		return err
	}

	for _, snapshotID := range list {
		err := fsck_snapshot(cmd, s, snapshotID)

		if err != nil {
			return err
		}
	}

	if !cmd.Orphaned {
		return nil
	}

	debug("starting orphaned check\n")

	l := []struct {
		desc string
		tpe  backend.Type
		list *restic.BlobList
	}{
		{"data blob", backend.Data, cmd.o_data},
		{"tree", backend.Tree, cmd.o_trees},
		{"maps", backend.Map, cmd.o_maps},
	}

	for _, d := range l {
		debug("checking for orphaned %v\n", d.desc)

		blobs, err := s.List(d.tpe)
		if err != nil {
			return err
		}

		for _, id := range blobs {
			_, err := d.list.Find(restic.Blob{ID: id})
			if err == restic.ErrBlobNotFound {
				if !cmd.RemoveOrphaned {
					fmt.Printf("orphaned %v %v\n", d.desc, id)
					continue
				}

				fmt.Printf("removing orphaned %v %v\n", d.desc, id)
				err := s.Remove(d.tpe, id)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}
