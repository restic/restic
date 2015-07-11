package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/restic/restic/checker"
)

type CmdCheck struct {
	ReadData bool `          long:"read-data"       description:"Read data blobs" default:"false"`

	global *GlobalOptions
}

func init() {
	_, err := parser.AddCommand("check",
		"check the repository",
		"The check command check the integrity and consistency of the repository",
		&CmdCheck{global: &globalOpts})
	if err != nil {
		panic(err)
	}
}

func (cmd CmdCheck) Usage() string {
	return "[check-options]"
}

func (cmd CmdCheck) Execute(args []string) error {
	if len(args) != 0 {
		return errors.New("check has no arguments")
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

	checker := checker.New(repo)

	cmd.global.Verbosef("Load indexes\n")
	if err = checker.LoadIndex(); err != nil {
		return err
	}

	done := make(chan struct{})
	defer close(done)

	errorsFound := false
	errChan := make(chan error)

	cmd.global.Verbosef("Check all packs\n")
	go checker.Packs(errChan, done)

	for err := range errChan {
		errorsFound = true
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
	}

	cmd.global.Verbosef("Check snapshots, trees and blobs\n")
	errChan = make(chan error)
	go checker.Structure(errChan, done)

	for err := range errChan {
		errorsFound = true
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
	}

	if errorsFound {
		return errors.New("repository contains errors")
	}
	return nil
}
