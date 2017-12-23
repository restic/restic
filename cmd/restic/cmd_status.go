package main

import (
	"context"

	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/spf13/cobra"
)

var cmdStatus = &cobra.Command{
	Use:               "status",
	Short:             "Status of the repo",
	Long:              `The "status" command shows the status of the repository`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runStatus(globalOptions)
	},
}

func init() {
	cmdRoot.AddCommand(cmdStatus)
}

func runStatus(gopts GlobalOptions) error {
	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	if !gopts.NoLock {
		lock, err := lockRepo(repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	ctx, cancel := context.WithCancel(gopts.ctx)
	defer cancel()

	amountSnapshots := getSnapshotAmount(ctx, repo)
	amountKeys := getFileAmount(ctx, repo, restic.KeyFile)
	amountIndexes := getFileAmount(ctx, repo, restic.IndexFile)
	amountBlobs := getFileAmount(ctx, repo, restic.DataFile)

	Printf("Snapshots: %d\n", amountSnapshots)
	Printf("Keys: %d\n", amountKeys)
	Printf("Indexes: %d\n", amountIndexes)
	Printf("Datafiles: %d\n", amountBlobs)

	return err
}

func getSnapshotAmount(ctx context.Context, repo *repository.Repository) int {
	var list restic.Snapshots
	for sn := range FindFilteredSnapshots(ctx, repo, "", []restic.TagList{}, []string{}, []string{}) {
		list = append(list, sn)
	}

	return len(list)
}

func getFileAmount(ctx context.Context, repo *repository.Repository, fileType restic.FileType) int {
	var amount int
	for range repo.List(ctx, fileType) {
		amount++
	}

	return amount
}
