package main

import (
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
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

Exit status is 0 if the command was successful, and non-zero if there was any error.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runList(cmd, globalOptions, args)
	},
}

func init() {
	cmdRoot.AddCommand(cmdList)
}

func runList(cmd *cobra.Command, opts GlobalOptions, args []string) error {
	if len(args) != 1 {
		return errors.Fatal("type not specified, usage: " + cmd.Use)
	}

	repo, err := OpenRepository(opts)
	if err != nil {
		return err
	}

	if !opts.NoLock {
		lock, err := lockRepo(opts.ctx, repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

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
		return repository.ForAllIndexes(opts.ctx, repo, func(id restic.ID, idx *repository.Index, oldFormat bool, err error) error {
			if err != nil {
				return err
			}
			for blobs := range idx.Each(opts.ctx) {
				Printf("%v %v\n", blobs.Type, blobs.ID)
			}
			return nil
		})
	default:
		return errors.Fatal("invalid type")
	}

	return repo.List(opts.ctx, t, func(id restic.ID, size int64) error {
		Printf("%s\n", id)
		return nil
	})
}
