package main

import (
	"restic"
	"restic/errors"

	"github.com/spf13/cobra"
)

var cmdList = &cobra.Command{
	Use:   "list [blobs|packs|index|snapshots|keys|locks]",
	Short: "list items in the repository",
	Long: `

`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runList(globalOptions, args)
	},
}

func init() {
	cmdRoot.AddCommand(cmdList)
}

func runList(opts GlobalOptions, args []string) error {
	if len(args) != 1 {
		return errors.Fatalf("type not specified")
	}

	repo, err := OpenRepository(opts)
	if err != nil {
		return err
	}

	if !opts.NoLock {
		lock, err := lockRepo(repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	var t restic.FileType
	switch args[0] {
	case "packs":
		t = restic.DataFile
	case "index":
		t = restic.IndexFile
	case "snapshots":
		t = restic.SnapshotFile
	case "keys":
		t = restic.KeyFile
	case "locks":
		t = restic.LockFile
	default:
		return errors.Fatal("invalid type")
	}

	for id := range repo.List(t, nil) {
		Printf("%s\n", id)
	}

	return nil
}
