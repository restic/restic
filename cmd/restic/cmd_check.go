package main

import (
	"io/ioutil"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/restic/restic/internal/cache"
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

EXIT STATUS
===========

Exit status is 0 if the command was successful, and non-zero if there was any error.
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
	f.StringVar(&checkOptions.ReadDataSubset, "read-data-subset", "", "read a `subset` of data packs, specified as 'n/t' for specific part, or either 'x%' or 'x.y%' or a size in bytes with suffixes k/K, m/M, g/G, t/T for a random subset")
	var ignored bool
	f.BoolVar(&ignored, "check-unused", false, "find unused blobs")
	err := f.MarkDeprecated("check-unused", "`--check-unused` is deprecated and will be ignored")
	if err != nil {
		// MarkDeprecated only returns an error when the flag is not found
		panic(err)
	}
	f.BoolVar(&checkOptions.WithCache, "with-cache", false, "use the cache")
}

func checkFlags(opts CheckOptions) error {
	if opts.ReadData && opts.ReadDataSubset != "" {
		return errors.Fatal("check flags --read-data and --read-data-subset cannot be used together")
	}
	if opts.ReadDataSubset != "" {
		dataSubset, err := stringToIntSlice(opts.ReadDataSubset)
		argumentError := errors.Fatal("check flag --read-data-subset has invalid value, please see documentation")
		if err == nil {
			if len(dataSubset) != 2 {
				return argumentError
			}
			if dataSubset[0] == 0 || dataSubset[1] == 0 || dataSubset[0] > dataSubset[1] {
				return errors.Fatal("check flag --read-data-subset=n/t values must be positive integers, and n <= t, e.g. --read-data-subset=1/2")
			}
			if dataSubset[1] > totalBucketsMax {
				return errors.Fatalf("check flag --read-data-subset=n/t t must be at most %d", totalBucketsMax)
			}
		} else if strings.HasSuffix(opts.ReadDataSubset, "%") {
			percentage, err := parsePercentage(opts.ReadDataSubset)
			if err != nil {
				return argumentError
			}

			if percentage <= 0.0 || percentage > 100.0 {
				return errors.Fatal(
					"check flag --read-data-subset=x% x must be above 0.0% and at most 100.0%")
			}

		} else {
			fileSize, err := parseSizeStr(opts.ReadDataSubset)
			if err != nil {
				return argumentError
			}
			if fileSize <= 0.0 {
				return errors.Fatal(
					"check flag --read-data-subset=n n must be above 0")
			}

		}
	}

	return nil
}

// See doReadData in runCheck below for why this is 256.
const totalBucketsMax = 256

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

// ParsePercentage parses a percentage string of the form "X%" where X is a float constant,
// and returns the value of that constant. It does not check the range of the value.
func parsePercentage(s string) (float64, error) {
	if !strings.HasSuffix(s, "%") {
		return 0, errors.Errorf(`parsePercentage: %q does not end in "%%"`, s)
	}
	s = s[:len(s)-1]

	p, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, errors.Errorf("parsePercentage: %v", err)
	}
	return p, nil
}

// prepareCheckCache configures a special cache directory for check.
//
//   - if --with-cache is specified, the default cache is used
//   - if the user explicitly requested --no-cache, we don't use any cache
//   - if the user provides --cache-dir, we use a cache in a temporary sub-directory of the specified directory and the sub-directory is deleted after the check
//   - by default, we use a cache in a temporary directory that is deleted after the check
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
	if cachedir == "" {
		cachedir = cache.EnvDir()
	}

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
		return errors.Fatal("the check command expects no arguments, only options - please see `restic help check` for usage and flags")
	}

	cleanup := prepareCheckCache(opts, &gopts)
	AddCleanupHandler(func(code int) (int, error) {
		cleanup()
		return code, nil
	})

	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	if !gopts.NoLock {
		Verbosef("create exclusive lock for repository\n")
		lock, err := lockRepoExclusive(gopts.ctx, repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	chkr := checker.New(repo, opts.CheckUnused)
	err = chkr.LoadSnapshots(gopts.ctx)
	if err != nil {
		return err
	}

	Verbosef("load indexes\n")
	hints, errs := chkr.LoadIndex(gopts.ctx)

	errorsFound := false
	suggestIndexRebuild := false
	mixedFound := false
	for _, hint := range hints {
		switch hint.(type) {
		case *checker.ErrDuplicatePacks, *checker.ErrOldIndexFormat:
			Printf("%v\n", hint)
			suggestIndexRebuild = true
		case *checker.ErrMixedPack:
			Printf("%v\n", hint)
			mixedFound = true
		default:
			Warnf("error: %v\n", hint)
			errorsFound = true
		}
	}

	if suggestIndexRebuild {
		Printf("This is non-critical, you can run `restic rebuild-index' to correct this\n")
	}
	if mixedFound {
		Printf("Mixed packs with tree and data blobs are non-critical, you can run `restic prune` to correct this.\n")
	}

	if len(errs) > 0 {
		for _, err := range errs {
			Warnf("error: %v\n", err)
		}
		return errors.Fatal("LoadIndex returned errors")
	}

	orphanedPacks := 0
	errChan := make(chan error)

	Verbosef("check all packs\n")
	go chkr.Packs(gopts.ctx, errChan)

	for err := range errChan {
		if checker.IsOrphanedPack(err) {
			orphanedPacks++
			Verbosef("%v\n", err)
		} else if _, ok := err.(*checker.ErrLegacyLayout); ok {
			Verbosef("repository still uses the S3 legacy layout\nPlease run `restic migrate s3legacy` to correct this.\n")
		} else {
			errorsFound = true
			Warnf("%v\n", err)
		}
	}

	if orphanedPacks > 0 {
		Verbosef("%d additional files were found in the repo, which likely contain duplicate data.\nThis is non-critical, you can run `restic prune` to correct this.\n", orphanedPacks)
	}

	Verbosef("check snapshots, trees and blobs\n")
	errChan = make(chan error)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		bar := newProgressMax(!gopts.Quiet, 0, "snapshots")
		defer bar.Done()
		chkr.Structure(gopts.ctx, bar, errChan)
	}()

	for err := range errChan {
		errorsFound = true
		if e, ok := err.(*checker.TreeError); ok {
			Warnf("error for tree %v:\n", e.ID.Str())
			for _, treeErr := range e.Errors {
				Warnf("  %v\n", treeErr)
			}
		} else {
			Warnf("error: %v\n", err)
		}
	}

	// Wait for the progress bar to be complete before printing more below.
	// Must happen after `errChan` is read from in the above loop to avoid
	// deadlocking in the case of errors.
	wg.Wait()

	if opts.CheckUnused {
		for _, id := range chkr.UnusedBlobs(gopts.ctx) {
			Verbosef("unused blob %v\n", id)
			errorsFound = true
		}
	}

	doReadData := func(packs map[restic.ID]int64) {
		packCount := uint64(len(packs))

		p := newProgressMax(!gopts.Quiet, packCount, "packs")
		errChan := make(chan error)

		go chkr.ReadPacks(gopts.ctx, packs, p, errChan)

		for err := range errChan {
			errorsFound = true
			Warnf("%v\n", err)
		}
		p.Done()
	}

	switch {
	case opts.ReadData:
		Verbosef("read all data\n")
		doReadData(selectPacksByBucket(chkr.GetPacks(), 1, 1))
	case opts.ReadDataSubset != "":
		var packs map[restic.ID]int64
		dataSubset, err := stringToIntSlice(opts.ReadDataSubset)
		if err == nil {
			bucket := dataSubset[0]
			totalBuckets := dataSubset[1]
			packs = selectPacksByBucket(chkr.GetPacks(), bucket, totalBuckets)
			packCount := uint64(len(packs))
			Verbosef("read group #%d of %d data packs (out of total %d packs in %d groups)\n", bucket, packCount, chkr.CountPacks(), totalBuckets)
		} else if strings.HasSuffix(opts.ReadDataSubset, "%") {
			percentage, err := parsePercentage(opts.ReadDataSubset)
			if err == nil {
				packs = selectRandomPacksByPercentage(chkr.GetPacks(), percentage)
				Verbosef("read %.1f%% of data packs\n", percentage)
			}
		} else {
			repoSize := int64(0)
			allPacks := chkr.GetPacks()
			for _, size := range allPacks {
				repoSize += size
			}
			if repoSize == 0 {
				return errors.Fatal("Cannot read from a repository having size 0")
			}
			subsetSize, _ := parseSizeStr(opts.ReadDataSubset)
			if subsetSize > repoSize {
				subsetSize = repoSize
			}
			packs = selectRandomPacksByFileSize(chkr.GetPacks(), subsetSize, repoSize)
			Verbosef("read %d bytes of data packs\n", subsetSize)
		}
		if packs == nil {
			return errors.Fatal("internal error: failed to select packs to check")
		}
		doReadData(packs)
	}

	if errorsFound {
		return errors.Fatal("repository contains errors")
	}

	Verbosef("no errors were found\n")

	return nil
}

// selectPacksByBucket selects subsets of packs by ranges of buckets.
func selectPacksByBucket(allPacks map[restic.ID]int64, bucket, totalBuckets uint) map[restic.ID]int64 {
	packs := make(map[restic.ID]int64)
	for pack, size := range allPacks {
		// If we ever check more than the first byte
		// of pack, update totalBucketsMax.
		if (uint(pack[0]) % totalBuckets) == (bucket - 1) {
			packs[pack] = size
		}
	}
	return packs
}

// selectRandomPacksByPercentage selects the given percentage of packs which are randomly choosen.
func selectRandomPacksByPercentage(allPacks map[restic.ID]int64, percentage float64) map[restic.ID]int64 {
	packCount := len(allPacks)
	packsToCheck := int(float64(packCount) * (percentage / 100.0))
	if packCount > 0 && packsToCheck < 1 {
		packsToCheck = 1
	}
	timeNs := time.Now().UnixNano()
	r := rand.New(rand.NewSource(timeNs))
	idx := r.Perm(packCount)

	var keys []restic.ID
	for k := range allPacks {
		keys = append(keys, k)
	}

	packs := make(map[restic.ID]int64)

	for i := 0; i < packsToCheck; i++ {
		id := keys[idx[i]]
		packs[id] = allPacks[id]
	}
	return packs
}

func selectRandomPacksByFileSize(allPacks map[restic.ID]int64, subsetSize int64, repoSize int64) map[restic.ID]int64 {
	subsetPercentage := (float64(subsetSize) / float64(repoSize)) * 100.0
	packs := selectRandomPacksByPercentage(allPacks, subsetPercentage)
	return packs
}
