package main

import (
	"errors"

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

	return nil
}
