package main

import (
	"errors"
	"fmt"

	"restic/backend"
	"restic/checker"
)

type CmdOptimize struct {
	global *GlobalOptions
}

func init() {
	_, err := parser.AddCommand("optimize",
		"optimize the repository",
		"The optimize command reorganizes the repository and removes uneeded data",
		&CmdOptimize{global: &globalOpts})
	if err != nil {
		panic(err)
	}
}

func (cmd CmdOptimize) Usage() string {
	return "[optimize-options]"
}

func (cmd CmdOptimize) Execute(args []string) error {
	if len(args) != 0 {
		return errors.New("optimize has no arguments")
	}

	repo, err := cmd.global.OpenRepository()
	if err != nil {
		return err
	}

	cmd.global.Verbosef("Create exclusive lock for repository\n")
	lock, err := lockRepoExclusive(repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	chkr := checker.New(repo)

	cmd.global.Verbosef("Load indexes\n")
	_, errs := chkr.LoadIndex()

	if len(errs) > 0 {
		for _, err := range errs {
			cmd.global.Warnf("error: %v\n", err)
		}
		return fmt.Errorf("LoadIndex returned errors")
	}

	done := make(chan struct{})
	errChan := make(chan error)
	go chkr.Structure(errChan, done)

	for err := range errChan {
		if e, ok := err.(checker.TreeError); ok {
			cmd.global.Warnf("error for tree %v:\n", e.ID.Str())
			for _, treeErr := range e.Errors {
				cmd.global.Warnf("  %v\n", treeErr)
			}
		} else {
			cmd.global.Warnf("error: %v\n", err)
		}
	}

	unusedBlobs := backend.NewIDSet(chkr.UnusedBlobs()...)
	cmd.global.Verbosef("%d unused blobs found, repacking...\n", len(unusedBlobs))

	repacker := checker.NewRepacker(repo, unusedBlobs)
	err = repacker.Repack()
	if err != nil {
		return err
	}

	cmd.global.Verbosef("repacking done\n")
	return nil
}
