package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"golang.org/x/crypto/ssh/terminal"

	"restic"
	"restic/checker"
	"restic/errors"
)

var cmdCheck = &cobra.Command{
	Use:   "check [flags]",
	Short: "check the repository for errors",
	Long: `
The "check" command tests the repository for errors and reports any errors it
finds. It can also be used to read all data and therefore simulate a restore.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCheck(checkOptions, globalOptions, args)
	},
}

// CheckOptions bundle all options for the 'check' command.
type CheckOptions struct {
	ReadData    bool
	CheckUnused bool
}

var checkOptions CheckOptions

func init() {
	cmdRoot.AddCommand(cmdCheck)

	f := cmdCheck.Flags()
	f.BoolVar(&checkOptions.ReadData, "read-data", false, "Read all data blobs")
	f.BoolVar(&checkOptions.CheckUnused, "check-unused", false, "Find unused blobs")
}

func newReadProgress(gopts GlobalOptions, todo restic.Stat) *restic.Progress {
	if gopts.Quiet {
		return nil
	}

	readProgress := restic.NewProgress()

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

		PrintProgress("%s", status)
	}

	readProgress.OnDone = func(s restic.Stat, d time.Duration, ticker bool) {
		fmt.Printf("\nduration: %s\n", formatDuration(d))
	}

	return readProgress
}

func runCheck(opts CheckOptions, gopts GlobalOptions, args []string) error {
	if len(args) != 0 {
		return errors.Fatal("check has no arguments")
	}

	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	if !gopts.NoLock {
		Verbosef("Create exclusive lock for repository\n")
		lock, err := lockRepoExclusive(repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	chkr := checker.New(repo)

	Verbosef("Load indexes\n")
	hints, errs := chkr.LoadIndex()

	dupFound := false
	for _, hint := range hints {
		Printf("%v\n", hint)
		if _, ok := hint.(checker.ErrDuplicatePacks); ok {
			dupFound = true
		}
	}

	if dupFound {
		Printf("\nrun `restic rebuild-index' to correct this\n")
	}

	if len(errs) > 0 {
		for _, err := range errs {
			Warnf("error: %v\n", err)
		}
		return errors.Fatal("LoadIndex returned errors")
	}

	done := make(chan struct{})
	defer close(done)

	errorsFound := false
	errChan := make(chan error)

	Verbosef("Check all packs\n")
	go chkr.Packs(errChan, done)

	for err := range errChan {
		errorsFound = true
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}

	Verbosef("Check snapshots, trees and blobs\n")
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

	if opts.CheckUnused {
		for _, id := range chkr.UnusedBlobs() {
			Verbosef("unused blob %v\n", id.Str())
			errorsFound = true
		}
	}

	if opts.ReadData {
		Verbosef("Read all data\n")

		p := newReadProgress(gopts, restic.Stat{Blobs: chkr.CountPacks()})
		errChan := make(chan error)

		go chkr.ReadData(p, errChan, done)

		for err := range errChan {
			errorsFound = true
			fmt.Fprintf(os.Stderr, "%v\n", err)
		}
	}

	if errorsFound {
		return errors.Fatal("repository contains errors")
	}
	return nil
}
