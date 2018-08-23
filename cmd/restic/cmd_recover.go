package main

import (
	"os"
	"time"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	"github.com/spf13/cobra"
)

var cmdRecover = &cobra.Command{
	Use:   "recover [flags]",
	Short: "Recover data from the repository",
	Long: `
The "recover" command build a new snapshot from all directories it can find in
the raw data of the repository. It can be used if, for example, a snapshot has
been removed by accident with "forget".
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRecover(globalOptions)
	},
}

func init() {
	cmdRoot.AddCommand(cmdRecover)
}

func runRecover(gopts GlobalOptions) error {
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}

	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	lock, err := lockRepo(repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	Verbosef("load index files\n")
	if err = repo.LoadIndex(gopts.ctx); err != nil {
		return err
	}

	// trees maps a tree ID to whether or not it is referenced by a different
	// tree. If it is not referenced, we have a root tree.
	trees := make(map[restic.ID]bool)

	for blob := range repo.Index().Each(gopts.ctx) {
		if blob.Blob.Type != restic.TreeBlob {
			continue
		}
		trees[blob.Blob.ID] = false
	}

	cur := 0
	max := len(trees)
	Verbosef("load %d trees\n\n", len(trees))

	for id := range trees {
		cur++
		Verbosef("\rtree (%v/%v)", cur, max)

		if !trees[id] {
			trees[id] = false
		}

		tree, err := repo.LoadTree(gopts.ctx, id)
		if err != nil {
			Warnf("unable to load tree %v: %v\n", id.Str(), err)
			continue
		}

		for _, node := range tree.Nodes {
			if node.Type != "dir" || node.Subtree == nil {
				continue
			}

			subtree := *node.Subtree
			trees[subtree] = true
		}
	}
	Verbosef("\ndone\n")

	roots := restic.NewIDSet()
	for id, seen := range trees {
		if seen {
			continue
		}

		roots.Insert(id)
	}

	Verbosef("found %d roots\n", len(roots))

	tree := restic.NewTree()
	for id := range roots {
		var subtreeID = id
		node := restic.Node{
			Type:       "dir",
			Name:       id.Str(),
			Mode:       0755,
			Subtree:    &subtreeID,
			AccessTime: time.Now(),
			ModTime:    time.Now(),
			ChangeTime: time.Now(),
		}
		tree.Insert(&node)
	}

	treeID, err := repo.SaveTree(gopts.ctx, tree)
	if err != nil {
		return errors.Fatalf("unable to save new tree to the repo: %v", err)
	}

	err = repo.Flush(gopts.ctx)
	if err != nil {
		return errors.Fatalf("unable to save blobs to the repo: %v", err)
	}

	err = repo.SaveIndex(gopts.ctx)
	if err != nil {
		return errors.Fatalf("unable to save new index to the repo: %v", err)
	}

	sn, err := restic.NewSnapshot([]string{"/recover"}, []string{}, hostname, time.Now())
	if err != nil {
		return errors.Fatalf("unable to save snapshot: %v", err)
	}

	sn.Tree = &treeID

	id, err := repo.SaveJSONUnpacked(gopts.ctx, restic.SnapshotFile, sn)
	if err != nil {
		return errors.Fatalf("unable to save snapshot: %v", err)
	}

	Printf("saved new snapshot %v\n", id.Str())

	return nil
}
