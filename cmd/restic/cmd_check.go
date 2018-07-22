package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/restic/restic/internal/checker"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
)

var cmdCheck = &cobra.Command{
	Use:   "check [flags]",
	Short: "Check the repository for errors",
	Long: `
The "check" command tests the repository for errors and reports any errors it
finds. It can also be used to read all data and therefore simulate a restore.

By default, the "check" command will always load all data directly from the
repository and not use a local cache.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCheck(checkOptions, globalOptions, args)
	},
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return checkFlags(checkOptions)
	},
}

// CheckOptions bundles all options for the 'check' command.
type CheckOptions struct {
	ReadData       bool
	ReadDataSubset string
	CheckUnused    bool
	WithCache      bool
}

var checkOptions CheckOptions

func init() {
	cmdRoot.AddCommand(cmdCheck)

	f := cmdCheck.Flags()
	f.BoolVar(&checkOptions.ReadData, "read-data", false, "read all data blobs")
	f.StringVar(&checkOptions.ReadDataSubset, "read-data-subset", "", "read subset n of m data packs (format: `n/m`)")
	f.BoolVar(&checkOptions.CheckUnused, "check-unused", false, "find unused blobs")
	f.BoolVar(&checkOptions.WithCache, "with-cache", false, "use the cache")
}

func checkFlags(opts CheckOptions) error {
	if opts.ReadData && opts.ReadDataSubset != "" {
		return errors.Fatalf("check flags --read-data and --read-data-subset cannot be used together")
	}
	if opts.ReadDataSubset != "" {
		dataSubset, err := stringToIntSlice(opts.ReadDataSubset)
		if err != nil || len(dataSubset) != 2 {
			return errors.Fatalf("check flag --read-data-subset must have two positive integer values, e.g. --read-data-subset=1/2")
		}
		if dataSubset[0] == 0 || dataSubset[1] == 0 || dataSubset[0] > dataSubset[1] {
			return errors.Fatalf("check flag --read-data-subset=n/t values must be positive integers, and n <= t, e.g. --read-data-subset=1/2")
		}
	}

	return nil
}

// stringToIntSlice converts string to []uint, using '/' as element separator
func stringToIntSlice(param string) (split []uint, err error) {
	if param == "" {
		return nil, nil
	}
	parts := strings.Split(param, "/")
	result := make([]uint, len(parts))
	for idx, part := range parts {
		uintval, err := strconv.ParseUint(part, 10, 0)
		if err != nil {
			return nil, err
		}
		result[idx] = uint(uintval)
	}
	return result, nil
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

		if w := stdoutTerminalWidth(); w > 0 {
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

// prepareCheckCache configures a special cache directory for check.
//
//  * if --with-cache is specified, the default cache is used
//  * if the user explicitly requested --no-cache, we don't use any cache
//  * if the user provides --cache-dir, we use a cache in a temporary sub-directory of the specified directory and the sub-directory is deleted after the check
//  * by default, we use a cache in a temporary directory that is deleted after the check
func prepareCheckCache(opts CheckOptions, gopts *GlobalOptions) (cleanup func()) {
	cleanup = func() {}
	if opts.WithCache {
		// use the default cache, no setup needed
		return cleanup
	}

	if gopts.NoCache {
		// don't use any cache, no setup needed
		return cleanup
	}

	cachedir := gopts.CacheDir

	// use a cache in a temporary directory
	tempdir, err := ioutil.TempDir(cachedir, "restic-check-cache-")
	if err != nil {
		// if an error occurs, don't use any cache
		Warnf("unable to create temporary directory for cache during check, disabling cache: %v\n", err)
		gopts.NoCache = true
		return cleanup
	}

	gopts.CacheDir = tempdir
	Verbosef("using temporary cache in %v\n", tempdir)

	cleanup = func() {
		err := fs.RemoveAll(tempdir)
		if err != nil {
			Warnf("error removing temporary cache directory: %v\n", err)
		}
	}

	return cleanup
}

func runCheck(opts CheckOptions, gopts GlobalOptions, args []string) error {
	if len(args) != 0 {
		return errors.Fatal("check has no arguments")
	}

	cleanup := prepareCheckCache(opts, &gopts)
	AddCleanupHandler(func() error {
		cleanup()
		return nil
	})

	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	if !gopts.NoLock {
		Verbosef("create exclusive lock for repository\n")
		lock, err := lockRepoExclusive(repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	chkr := checker.New(repo)

	Verbosef("load indexes\n")
	hints, errs := chkr.LoadIndex(gopts.ctx)

	dupFound := false
	for _, hint := range hints {
		Printf("%v\n", hint)
		if _, ok := hint.(checker.ErrDuplicatePacks); ok {
			dupFound = true
		}
	}

	if dupFound {
		Printf("This is non-critical, you can run `restic rebuild-index' to correct this\n")
	}

	if len(errs) > 0 {
		for _, err := range errs {
			Warnf("error: %v\n", err)
		}
		return errors.Fatal("LoadIndex returned errors")
	}

	errorsFound := false
	orphanedPacks := 0
	errChan := make(chan error)

	Verbosef("check all packs\n")
	go chkr.Packs(gopts.ctx, errChan)

	for err := range errChan {
		if checker.IsOrphanedPack(err) {
			orphanedPacks++
			Verbosef("%v\n", err)
			continue
		}
		errorsFound = true
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}

	if orphanedPacks > 0 {
		Verbosef("%d additional files were found in the repo, which likely contain duplicate data.\nYou can run `restic prune` to correct this.\n", orphanedPacks)
	}

	Verbosef("check snapshots, trees and blobs\n")
	errChan = make(chan error)
	go chkr.Structure(gopts.ctx, errChan)

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

	doReadData := func(bucket, totalBuckets uint) {
		packs := restic.IDSet{}
		for pack := range chkr.GetPacks() {
			if (uint(pack[0]) % totalBuckets) == (bucket - 1) {
				packs.Insert(pack)
			}
		}
		packCount := uint64(len(packs))

		if packCount < chkr.CountPacks() {
			Verbosef(fmt.Sprintf("read group #%d of %d data packs (out of total %d packs in %d groups)\n", bucket, packCount, chkr.CountPacks(), totalBuckets))
		} else {
			Verbosef("read all data\n")
		}

		p := newReadProgress(gopts, restic.Stat{Blobs: packCount})
		errChan := make(chan error)

		go chkr.ReadPacks(gopts.ctx, packs, p, errChan)

		for err := range errChan {
			errorsFound = true
			fmt.Fprintf(os.Stderr, "%v\n", err)
		}
	}

	switch {
	case opts.ReadData:
		doReadData(1, 1)
	case opts.ReadDataSubset != "":
		dataSubset, _ := stringToIntSlice(opts.ReadDataSubset)
		doReadData(dataSubset[0], dataSubset[1])
	}

	if errorsFound {
		return errors.Fatal("repository contains errors")
	}

	Verbosef("no errors were found\n")

	return nil
}
