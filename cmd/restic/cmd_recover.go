package main

import (
	"context"
	"os"
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

var cmdRecover = &cobra.Command{
	Use:   "recover [flags]",
	Short: "Recover data from the repository not referenced by snapshots",
	Long: `
The "recover" command builds a new snapshot from all directories it can find in
the raw data of the repository which are not referenced in an existing snapshot.
It can be used if, for example, a snapshot has been removed by accident with "forget".

EXIT STATUS
===========

Exit status is 0 if the command was successful, and non-zero if there was any error.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRecover(cmd.Context(), globalOptions)
	},
}

func init() {
	cmdRoot.AddCommand(cmdRecover)
}

func runRecover(ctx context.Context, gopts GlobalOptions) error {
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}

	repo, err := OpenRepository(ctx, gopts)
	if err != nil {
		return err
	}

	lock, ctx, err := lockRepo(ctx, repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	snapshotLister, err := backend.MemorizeList(ctx, repo.Backend(), restic.SnapshotFile)
	if err != nil {
		return err
	}

	Verbosef("load index files\n")
	if err = repo.LoadIndex(ctx); err != nil {
		return err
	}

	// trees maps a tree ID to whether or not it is referenced by a different
	// tree. If it is not referenced, we have a root tree.
	trees := make(map[restic.ID]bool)

	repo.Index().Each(ctx, func(blob restic.PackedBlob) {
		if blob.Type == restic.TreeBlob {
			trees[blob.Blob.ID] = false
		}
	})

	Verbosef("load %d trees\n", len(trees))
	bar := newProgressMax(!gopts.Quiet, uint64(len(trees)), "trees loaded")
	for id := range trees {
		tree, err := restic.LoadTree(ctx, repo, id)
		if err != nil {
			Warnf("unable to load tree %v: %v\n", id.Str(), err)
			continue
		}

		for _, node := range tree.Nodes {
			if node.Type == "dir" && node.Subtree != nil {
				trees[*node.Subtree] = true
			}
		}
		bar.Add(1)
	}
	bar.Done()

	Verbosef("load snapshots\n")
	err = restic.ForAllSnapshots(ctx, snapshotLister, repo, nil, func(id restic.ID, sn *restic.Snapshot, err error) error {
		trees[*sn.Tree] = true
		return nil
	})
	if err != nil {
		return err
	}
	Verbosef("done\n")

	roots := restic.NewIDSet()
	for id, seen := range trees {
		if !seen {
			Verboseff("found root tree %v\n", id.Str())
			roots.Insert(id)
		}
	}
	Printf("\nfound %d unreferenced roots\n", len(roots))

	if len(roots) == 0 {
		Verbosef("no snapshot to write.\n")
		return nil
	}

	tree := restic.NewTree(len(roots))
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
		err := tree.Insert(&node)
		if err != nil {
			return err
		}
	}

	wg, wgCtx := errgroup.WithContext(ctx)
	repo.StartPackUploader(wgCtx, wg)

	var treeID restic.ID
	wg.Go(func() error {
		var err error
		treeID, err = restic.SaveTree(wgCtx, repo, tree)
		if err != nil {
			return errors.Fatalf("unable to save new tree to the repository: %v", err)
		}

		err = repo.Flush(wgCtx)
		if err != nil {
			return errors.Fatalf("unable to save blobs to the repository: %v", err)
		}
		return nil
	})
	err = wg.Wait()
	if err != nil {
		return err
	}

	return createSnapshot(ctx, "/recover", hostname, []string{"recovered"}, repo, &treeID)

}

func createSnapshot(ctx context.Context, name, hostname string, tags []string, repo restic.Repository, tree *restic.ID) error {
	sn, err := restic.NewSnapshot([]string{name}, tags, hostname, time.Now())
	if err != nil {
		return errors.Fatalf("unable to save snapshot: %v", err)
	}

	sn.Tree = tree

	id, err := restic.SaveSnapshot(ctx, repo, sn)
	if err != nil {
		return errors.Fatalf("unable to save snapshot: %v", err)
	}

	Printf("saved new snapshot %v\n", id.Str())
	return nil
}
