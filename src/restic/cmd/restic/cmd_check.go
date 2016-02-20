package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/crypto/ssh/terminal"

	"restic"
	"restic/checker"
)

type CmdCheck struct {
	ReadData    bool `long:"read-data"    default:"false" description:"Read data blobs"`
	CheckUnused bool `long:"check-unused" default:"false" description:"Check for unused blobs"`

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

func (cmd CmdCheck) newReadProgress(todo restic.Stat) *restic.Progress {
	if !cmd.global.ShowProgress() {
		return nil
	}

	readProgress := restic.NewProgress(time.Second)

	readProgress.OnUpdate = func(s restic.Stat, d time.Duration, ticker bool) {
		status := fmt.Sprintf("[%s] %s  %d / %d items",
			formatDuration(d),
			formatPercent(s.Blobs, todo.Blobs),
			s.Blobs, todo.Blobs)

		w, _, err := terminal.GetSize(int(os.Stdout.Fd()))
		if err == nil {
			if len(status) > w {
				max := w - len(status) - 4
				status = status[:max] + "... "
			}
		}

		fmt.Printf("\x1b[2K%s\r", status)
	}

	readProgress.OnDone = func(s restic.Stat, d time.Duration, ticker bool) {
		fmt.Printf("\nduration: %s\n", formatDuration(d))
	}

	return readProgress
}

func (cmd CmdCheck) Execute(args []string) error {
	if len(args) != 0 {
		return errors.New("check has no arguments")
	}

	repo, err := cmd.global.OpenRepository()
	if err != nil {
		return err
	}

	if !cmd.global.NoLock {
		cmd.global.Verbosef("Create exclusive lock for repository\n")
		lock, err := lockRepoExclusive(repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	chkr := checker.New(repo)

	cmd.global.Verbosef("Load indexes\n")
	hints, errs := chkr.LoadIndex()

	dupFound := false
	for _, hint := range hints {
		cmd.global.Printf("%v\n", hint)
		if _, ok := hint.(checker.ErrDuplicatePacks); ok {
			dupFound = true
		}
	}

	if dupFound {
		cmd.global.Printf("\nrun `restic rebuild-index' to correct this\n")
	}

	if len(errs) > 0 {
		for _, err := range errs {
			cmd.global.Warnf("error: %v\n", err)
		}
		return fmt.Errorf("LoadIndex returned errors")
	}

	done := make(chan struct{})
	defer close(done)

	errorsFound := false
	errChan := make(chan error)

	cmd.global.Verbosef("Check all packs\n")
	go chkr.Packs(errChan, done)

	for err := range errChan {
		errorsFound = true
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}

	cmd.global.Verbosef("Check snapshots, trees and blobs\n")
	errChan = make(chan error)
	go chkr.Structure(errChan, done)

	for err := range errChan {
		errorsFound = true
		if e, ok := err.(checker.TreeError); ok {
			fmt.Fprintf(os.Stderr, "error for tree %v:\n", e.ID.Str())
			for _, treeErr := range e.Errors {
				fmt.Fprintf(os.Stderr, "  %v\n", treeErr)
			}
		} else {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
	}

	if cmd.CheckUnused {
		for _, id := range chkr.UnusedBlobs() {
			cmd.global.Verbosef("unused blob %v\n", id.Str())
			errorsFound = true
		}
	}

	if cmd.ReadData {
		cmd.global.Verbosef("Read all data\n")

		p := cmd.newReadProgress(restic.Stat{Blobs: chkr.CountPacks()})
		errChan := make(chan error)

		go chkr.ReadData(p, errChan, done)

		for err := range errChan {
			errorsFound = true
			fmt.Fprintf(os.Stderr, "%v\n", err)
		}
	}

	if errorsFound {
		return errors.New("repository contains errors")
	}
	return nil
}
