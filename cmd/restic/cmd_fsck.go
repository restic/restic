package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/crypto"
	"github.com/restic/restic/debug"
	"github.com/restic/restic/pack"
	"github.com/restic/restic/repository"
)

type CmdFsck struct {
	CheckData      bool   `          long:"check-data"      description:"Read data blobs" default:"false"`
	Snapshot       string `short:"s" long:"snapshot"        description:"Only check this snapshot"`
	Orphaned       bool   `short:"o" long:"orphaned"        description:"Check for orphaned blobs"`
	RemoveOrphaned bool   `short:"r" long:"remove-orphaned" description:"Remove orphaned blobs (implies -o)"`

	global *GlobalOptions

	// lists checking for orphaned blobs
	o_data  *backend.IDSet
	o_trees *backend.IDSet
}

func init() {
	_, err := parser.AddCommand("fsck",
		"check the repository",
		"The fsck command check the integrity and consistency of the repository",
		&CmdFsck{global: &globalOpts})
	if err != nil {
		panic(err)
	}
}

func fsckFile(global CmdFsck, repo *repository.Repository, IDs []backend.ID) (uint64, error) {
	debug.Log("restic.fsckFile", "checking file %v", IDs)
	var bytes uint64

	for _, id := range IDs {
		debug.Log("restic.fsck", "  checking data blob %v\n", id)

		// test if blob is in the index
		packID, tpe, _, length, err := repo.Index().Lookup(id)
		if err != nil {
			return 0, fmt.Errorf("storage for blob %v (%v) not found", id, tpe)
		}

		bytes += uint64(length - crypto.Extension)
		debug.Log("restic.fsck", "  blob found in pack %v\n", packID)

		if global.CheckData {
			// load content
			_, err := repo.LoadBlob(pack.Data, id)
			if err != nil {
				return 0, err
			}
		} else {
			// test if pack for data blob is there
			ok, err := repo.Backend().Test(backend.Data, packID.String())
			if err != nil {
				return 0, err
			}

			if !ok {
				return 0, fmt.Errorf("data blob %v not found", id)
			}
		}

		// if orphan check is active, record storage id
		if global.o_data != nil {
			debug.Log("restic.fsck", "  recording blob %v as used\n", id)
			global.o_data.Insert(id)
		}
	}

	return bytes, nil
}

func fsckTree(global CmdFsck, repo *repository.Repository, id backend.ID) error {
	debug.Log("restic.fsckTree", "checking tree %v", id.Str())

	tree, err := restic.LoadTree(repo, id)
	if err != nil {
		return err
	}

	// if orphan check is active, record storage id
	if global.o_trees != nil {
		// add ID to list
		global.o_trees.Insert(id)
	}

	var firstErr error

	seenIDs := backend.NewIDSet()

	for i, node := range tree.Nodes {
		if node.Name == "" {
			return fmt.Errorf("node %v of tree %v has no name", i, id.Str())
		}

		if node.Type == "" {
			return fmt.Errorf("node %q of tree %v has no type", node.Name, id.Str())
		}

		switch node.Type {
		case "file":
			if node.Content == nil {
				debug.Log("restic.fsckTree", "file node %q of tree %v has no content: %v", node.Name, id, node)
				return fmt.Errorf("file node %q of tree %v has no content: %v", node.Name, id, node)
			}

			if node.Content == nil && node.Error == "" {
				debug.Log("restic.fsckTree", "file node %q of tree %v has no content", node.Name, id)
				return fmt.Errorf("file node %q of tree %v has no content", node.Name, id)
			}

			// record ids
			for _, id := range node.Content {
				seenIDs.Insert(id)
			}

			debug.Log("restic.fsckTree", "check file %v (%v)", node.Name, id.Str())
			bytes, err := fsckFile(global, repo, node.Content)
			if err != nil {
				return err
			}

			if bytes != node.Size {
				debug.Log("restic.fsckTree", "file node %q of tree %v has size %d, but only %d bytes could be found", node.Name, id, node.Size, bytes)
				return fmt.Errorf("file node %q of tree %v has size %d, but only %d bytes could be found", node.Name, id, node.Size, bytes)
			}
		case "dir":
			if node.Subtree == nil {
				return fmt.Errorf("dir node %q of tree %v has no subtree", node.Name, id)
			}

			// record id
			seenIDs.Insert(node.Subtree)

			err = fsckTree(global, repo, node.Subtree)
			if err != nil {
				firstErr = err
				fmt.Fprintf(os.Stderr, "%v\n", err)
			}
		}
	}

	// check map for unused ids
	// for _, id := range tree.Map.IDs() {
	// 	if seenIDs.Find(id) != nil {
	// 		return fmt.Errorf("tree %v: map contains unused ID %v", id, id)
	// 	}
	// }

	return firstErr
}

func fsckSnapshot(global CmdFsck, repo *repository.Repository, id backend.ID) error {
	debug.Log("restic.fsck", "checking snapshot %v\n", id)

	sn, err := restic.LoadSnapshot(repo, id)
	if err != nil {
		return fmt.Errorf("loading snapshot %v failed: %v", id, err)
	}

	err = fsckTree(global, repo, sn.Tree)
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

	repo, err := cmd.global.OpenRepository()
	if err != nil {
		return err
	}

	lock, err := restic.NewExclusiveLock(repo)
	defer lock.Unlock()
	if err != nil {
		return err
	}

	err = repo.LoadIndex()
	if err != nil {
		return err
	}

	if cmd.Snapshot != "" {
		id, err := restic.FindSnapshot(repo, cmd.Snapshot)
		if err != nil {
			return fmt.Errorf("invalid id %q: %v", cmd.Snapshot, err)
		}

		err = fsckSnapshot(cmd, repo, id)
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
	for id := range repo.List(backend.Snapshot, done) {
		err = fsckSnapshot(cmd, repo, id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "check for snapshot %v failed\n", id)
			firstErr = err
		}
	}

	if !cmd.Orphaned {
		return firstErr
	}

	debug.Log("restic.fsck", "starting orphaned check\n")

	cnt := make(map[pack.BlobType]*backend.IDSet)
	cnt[pack.Data] = cmd.o_data
	cnt[pack.Tree] = cmd.o_trees

	for blob := range repo.Index().Each(done) {
		debug.Log("restic.fsck", "checking %v blob %v\n", blob.Type, blob.ID)

		err = cnt[blob.Type].Find(blob.ID)
		if err != nil {
			debug.Log("restic.fsck", "  blob %v is orphaned\n", blob.ID)

			if !cmd.RemoveOrphaned {
				fmt.Printf("orphaned %v blob %v\n", blob.Type, blob.ID)
				continue
			}

			fmt.Printf("removing orphaned %v blob %v\n", blob.Type, blob.ID)
			// err := s.Remove(d.tpe, name)
			// if err != nil {
			// 	return err
			// }
			return errors.New("not implemented")
		}
	}

	return firstErr
}
