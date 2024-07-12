package main

import (
	"context"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/restic/restic/internal/backend/cache"
	"github.com/restic/restic/internal/checker"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"
	"github.com/restic/restic/internal/ui/termstatus"
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

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
Exit status is 10 if the repository does not exist.
Exit status is 11 if the repository is already locked.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		term, cancel := setupTermstatus()
		defer cancel()
		return runCheck(cmd.Context(), checkOptions, globalOptions, args, term)
	},
	PreRunE: func(_ *cobra.Command, _ []string) error {
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
	f.BoolVar(&checkOptions.WithCache, "with-cache", false, "use existing cache, only read uncached data from repository")
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
			fileSize, err := ui.ParseBytes(opts.ReadDataSubset)
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
func prepareCheckCache(opts CheckOptions, gopts *GlobalOptions, printer progress.Printer) (cleanup func()) {
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

	if cachedir != "" {
		// use a cache in a temporary directory
		err := os.MkdirAll(cachedir, 0755)
		if err != nil {
			Warnf("unable to create cache directory %s, disabling cache: %v\n", cachedir, err)
			gopts.NoCache = true
			return cleanup
		}
	}
	tempdir, err := os.MkdirTemp(cachedir, "restic-check-cache-")
	if err != nil {
		// if an error occurs, don't use any cache
		printer.E("unable to create temporary directory for cache during check, disabling cache: %v\n", err)
		gopts.NoCache = true
		return cleanup
	}

	gopts.CacheDir = tempdir
	printer.P("using temporary cache in %v\n", tempdir)

	cleanup = func() {
		err := fs.RemoveAll(tempdir)
		if err != nil {
			printer.E("error removing temporary cache directory: %v\n", err)
		}
	}

	return cleanup
}

func runCheck(ctx context.Context, opts CheckOptions, gopts GlobalOptions, args []string, term *termstatus.Terminal) error {
	if len(args) != 0 {
		return errors.Fatal("the check command expects no arguments, only options - please see `restic help check` for usage and flags")
	}

	printer := newTerminalProgressPrinter(gopts.verbosity, term)

	cleanup := prepareCheckCache(opts, &gopts, printer)
	defer cleanup()

	if !gopts.NoLock {
		printer.P("create exclusive lock for repository\n")
	}
	ctx, repo, unlock, err := openWithExclusiveLock(ctx, gopts, gopts.NoLock)
	if err != nil {
		return err
	}
	defer unlock()

	chkr := checker.New(repo, opts.CheckUnused)
	err = chkr.LoadSnapshots(ctx)
	if err != nil {
		return err
	}

	printer.P("load indexes\n")
	bar := newIndexTerminalProgress(gopts.Quiet, gopts.JSON, term)
	hints, errs := chkr.LoadIndex(ctx, bar)
	if ctx.Err() != nil {
		return ctx.Err()
	}

	errorsFound := false
	suggestIndexRebuild := false
	suggestLegacyIndexRebuild := false
	mixedFound := false
	for _, hint := range hints {
		switch hint.(type) {
		case *checker.ErrDuplicatePacks:
			term.Print(hint.Error())
			suggestIndexRebuild = true
		case *checker.ErrOldIndexFormat:
			printer.E("error: %v\n", hint)
			suggestLegacyIndexRebuild = true
			errorsFound = true
		case *checker.ErrMixedPack:
			term.Print(hint.Error())
			mixedFound = true
		default:
			printer.E("error: %v\n", hint)
			errorsFound = true
		}
	}

	if suggestIndexRebuild {
		term.Print("Duplicate packs are non-critical, you can run `restic repair index' to correct this.\n")
	}
	if suggestLegacyIndexRebuild {
		printer.E("error: Found indexes using the legacy format, you must run `restic repair index' to correct this.\n")
	}
	if mixedFound {
		term.Print("Mixed packs with tree and data blobs are non-critical, you can run `restic prune` to correct this.\n")
	}

	if len(errs) > 0 {
		for _, err := range errs {
			printer.E("error: %v\n", err)
		}

		printer.E("\nThe repository index is damaged and must be repaired. You must run `restic repair index' to correct this.\n\n")
		return errors.Fatal("repository contains errors")
	}

	orphanedPacks := 0
	errChan := make(chan error)
	salvagePacks := restic.NewIDSet()

	printer.P("check all packs\n")
	go chkr.Packs(ctx, errChan)

	for err := range errChan {
		var packErr *checker.PackError
		if errors.As(err, &packErr) {
			if packErr.Orphaned {
				orphanedPacks++
				printer.V("%v\n", err)
			} else {
				if packErr.Truncated {
					salvagePacks.Insert(packErr.ID)
				}
				errorsFound = true
				printer.E("%v\n", err)
			}
		} else if err == checker.ErrLegacyLayout {
			errorsFound = true
			printer.E("error: repository still uses the S3 legacy layout\nYou must run `restic migrate s3legacy` to correct this.\n")
		} else {
			errorsFound = true
			printer.E("%v\n", err)
		}
	}

	if orphanedPacks > 0 && !errorsFound {
		// hide notice if repository is damaged
		printer.P("%d additional files were found in the repo, which likely contain duplicate data.\nThis is non-critical, you can run `restic prune` to correct this.\n", orphanedPacks)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	printer.P("check snapshots, trees and blobs\n")
	errChan = make(chan error)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		bar := newTerminalProgressMax(!gopts.Quiet, 0, "snapshots", term)
		defer bar.Done()
		chkr.Structure(ctx, bar, errChan)
	}()

	for err := range errChan {
		errorsFound = true
		if e, ok := err.(*checker.TreeError); ok {
			printer.E("error for tree %v:\n", e.ID.Str())
			for _, treeErr := range e.Errors {
				printer.E("  %v\n", treeErr)
			}
		} else {
			printer.E("error: %v\n", err)
		}
	}

	// Wait for the progress bar to be complete before printing more below.
	// Must happen after `errChan` is read from in the above loop to avoid
	// deadlocking in the case of errors.
	wg.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if opts.CheckUnused {
		unused, err := chkr.UnusedBlobs(ctx)
		if err != nil {
			return err
		}
		for _, id := range unused {
			printer.P("unused blob %v\n", id)
			errorsFound = true
		}
	}

	doReadData := func(packs map[restic.ID]int64) {
		packCount := uint64(len(packs))

		p := newTerminalProgressMax(!gopts.Quiet, packCount, "packs", term)
		errChan := make(chan error)

		go chkr.ReadPacks(ctx, packs, p, errChan)

		for err := range errChan {
			errorsFound = true
			printer.E("%v\n", err)
			if err, ok := err.(*repository.ErrPackData); ok {
				salvagePacks.Insert(err.PackID)
			}
		}
		p.Done()
	}

	switch {
	case opts.ReadData:
		printer.P("read all data\n")
		doReadData(selectPacksByBucket(chkr.GetPacks(), 1, 1))
	case opts.ReadDataSubset != "":
		var packs map[restic.ID]int64
		dataSubset, err := stringToIntSlice(opts.ReadDataSubset)
		if err == nil {
			bucket := dataSubset[0]
			totalBuckets := dataSubset[1]
			packs = selectPacksByBucket(chkr.GetPacks(), bucket, totalBuckets)
			packCount := uint64(len(packs))
			printer.P("read group #%d of %d data packs (out of total %d packs in %d groups)\n", bucket, packCount, chkr.CountPacks(), totalBuckets)
		} else if strings.HasSuffix(opts.ReadDataSubset, "%") {
			percentage, err := parsePercentage(opts.ReadDataSubset)
			if err == nil {
				packs = selectRandomPacksByPercentage(chkr.GetPacks(), percentage)
				printer.P("read %.1f%% of data packs\n", percentage)
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
			subsetSize, _ := ui.ParseBytes(opts.ReadDataSubset)
			if subsetSize > repoSize {
				subsetSize = repoSize
			}
			packs = selectRandomPacksByFileSize(chkr.GetPacks(), subsetSize, repoSize)
			printer.P("read %d bytes of data packs\n", subsetSize)
		}
		if packs == nil {
			return errors.Fatal("internal error: failed to select packs to check")
		}
		doReadData(packs)
	}

	if len(salvagePacks) > 0 {
		printer.E("\nThe repository contains damaged pack files. These damaged files must be removed to repair the repository. This can be done using the following commands. Please read the troubleshooting guide at https://restic.readthedocs.io/en/stable/077_troubleshooting.html first.\n\n")
		var strIDs []string
		for id := range salvagePacks {
			strIDs = append(strIDs, id.String())
		}
		printer.E("restic repair packs %v\nrestic repair snapshots --forget\n\n", strings.Join(strIDs, " "))
		printer.E("Damaged pack files can be caused by backend problems, hardware problems or bugs in restic. Please open an issue at https://github.com/restic/restic/issues/new/choose for further troubleshooting!\n")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	if errorsFound {
		if len(salvagePacks) == 0 {
			printer.E("\nThe repository is damaged and must be repaired. Please follow the troubleshooting guide at https://restic.readthedocs.io/en/stable/077_troubleshooting.html .\n\n")
		}
		return errors.Fatal("repository contains errors")
	}
	printer.P("no errors were found\n")

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

// selectRandomPacksByPercentage selects the given percentage of packs which are randomly chosen.
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
