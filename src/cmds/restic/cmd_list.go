package main

import "restic"

type CmdList struct {
	global *GlobalOptions
}

func init() {
	_, err := parser.AddCommand("list",
		"lists data",
		"The list command lists structures or data of a repository",
		&CmdList{global: &globalOpts})
	if err != nil {
		panic(err)
	}
}

func (cmd CmdList) Usage() string {
	return "[blobs|packs|index|snapshots|keys|locks]"
}

func (cmd CmdList) Execute(args []string) error {
	if len(args) != 1 {
		return restic.Fatalf("type not specified, Usage: %s", cmd.Usage())
	}

	repo, err := cmd.global.OpenRepository()
	if err != nil {
		return err
	}

	if !cmd.global.NoLock {
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
		return restic.Fatal("invalid type")
	}

	for id := range repo.List(t, nil) {
		cmd.global.Printf("%s\n", id)
	}

	return nil
}
