package main

import (
	"context"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository/index"
	"github.com/restic/restic/internal/restic"

	"github.com/spf13/cobra"
)

var cmdList = &cobra.Command{
	Use:   "list [flags] [blobs|packs|index|snapshots|keys|locks]",
	Short: "List objects in the repository",
	Long: `
The "list" command allows listing objects in the repository based on type.

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
Exit status is 10 if the repository does not exist.
Exit status is 11 if the repository is already locked.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runList(cmd.Context(), globalOptions, args)
	},
}

func init() {
	cmdRoot.AddCommand(cmdList)
}

func runList(ctx context.Context, gopts GlobalOptions, args []string) error {
	if len(args) != 1 {
		return errors.Fatal("type not specified")
	}

	ctx, repo, unlock, err := openWithReadLock(ctx, gopts, gopts.NoLock || args[0] == "locks")
	if err != nil {
		return err
	}
	defer unlock()

	var t restic.FileType
	switch args[0] {
	case "packs":
		t = restic.PackFile
	case "index":
		t = restic.IndexFile
	case "snapshots":
		t = restic.SnapshotFile
	case "keys":
		t = restic.KeyFile
	case "locks":
		t = restic.LockFile
	case "blobs":
		return index.ForAllIndexes(ctx, repo, repo, func(_ restic.ID, idx *index.Index, _ bool, err error) error {
			if err != nil {
				return err
			}
			return idx.Each(ctx, func(blobs restic.PackedBlob) {
				Printf("%v %v\n", blobs.Type, blobs.ID)
			})
		})
	default:
		return errors.Fatal("invalid type")
	}

	return repo.List(ctx, t, func(id restic.ID, _ int64) error {
		Printf("%s\n", id)
		return nil
	})
}
