package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/restic/restic/internal/restic"
	"github.com/spf13/cobra"
)

var cmdStats = &cobra.Command{
	Use:   "stats",
	Short: "Scan the repository and show basic statistics",
	Long: `
The "stats" command walks all snapshots in a repository and accumulates
statistics about the data stored therein. It reports on the number of
unique files and their sizes.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runStats(globalOptions, args)
	},
}

func init() {
	cmdRoot.AddCommand(cmdStats)
}

func runStats(gopts GlobalOptions, args []string) error {
	ctx, cancel := context.WithCancel(gopts.ctx)
	defer cancel()

	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	if err = repo.LoadIndex(ctx); err != nil {
		return err
	}

	if !gopts.NoLock {
		lock, err := lockRepo(repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	// create a container for the stats, and other state
	// needed while walking the trees
	stats := &statsContainer{idSet: restic.NewIDSet()}

	// iterate every snapshot in the repo
	err = repo.List(ctx, restic.SnapshotFile, func(snapshotID restic.ID, size int64) error {
		snapshot, err := restic.LoadSnapshot(ctx, repo, snapshotID)
		if err != nil {
			return fmt.Errorf("Error loading snapshot %s: %v", snapshotID.Str(), err)
		}
		if snapshot.Tree == nil {
			return fmt.Errorf("snapshot %s has nil tree", snapshot.ID().Str())
		}

		err = walkTree(ctx, repo, *snapshot.Tree, stats)
		if err != nil {
			return fmt.Errorf("walking tree %s: %v", *snapshot.Tree, err)
		}

		return nil
	})

	if gopts.JSON {
		err = json.NewEncoder(os.Stdout).Encode(stats)
		if err != nil {
			return fmt.Errorf("encoding output: %v", err)
		}
		return nil
	}

	Printf("   Cumulative Original Size:   %-5s\n", formatBytes(stats.TotalOriginalSize))
	Printf("  Total Original File Count:   %d\n", stats.TotalCount)
	return nil
}

func walkTree(ctx context.Context, repo restic.Repository, treeID restic.ID, stats *statsContainer) error {
	if stats.idSet.Has(treeID) {
		return nil
	}
	stats.idSet.Insert(treeID)

	tree, err := repo.LoadTree(ctx, treeID)
	if err != nil {
		return fmt.Errorf("loading tree: %v", err)
	}

	for _, node := range tree.Nodes {
		// update our stats to account for this node
		stats.TotalOriginalSize += node.Size
		stats.TotalCount++

		if node.Subtree != nil {
			err = walkTree(ctx, repo, *node.Subtree, stats)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// statsContainer holds information during a walk of a repository
// to collect information about it, as well as state needed
// for a successful and efficient walk.
type statsContainer struct {
	TotalCount        uint64 `json:"total_count"`
	TotalOriginalSize uint64 `json:"total_original_size"`
	idSet             restic.IDSet
}
