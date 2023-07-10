package main

import (
	"context"
	"math"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/index"
	"github.com/restic/restic/internal/pack"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"

	"github.com/spf13/cobra"
)

var errorIndexIncomplete = errors.Fatal("index is not complete")
var errorPacksMissing = errors.Fatal("packs from index missing in repo")
var errorSizeNotMatching = errors.Fatal("pack size does not match calculated size from index")

var cmdPrune = &cobra.Command{
	Use:   "prune [flags]",
	Short: "Remove unneeded data from the repository",
	Long: `
The "prune" command checks the repository and removes data that is not
referenced and therefore not needed any more.

EXIT STATUS
===========

Exit status is 0 if the command was successful, and non-zero if there was any error.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPrune(cmd.Context(), pruneOptions, globalOptions)
	},
}

// PruneOptions collects all options for the cleanup command.
type PruneOptions struct {
	DryRun                bool
	UnsafeNoSpaceRecovery string

	unsafeRecovery bool

	MaxUnused      string
	maxUnusedBytes func(used uint64) (unused uint64) // calculates the number of unused bytes after repacking, according to MaxUnused

	MaxRepackSize  string
	MaxRepackBytes uint64

	RepackCachableOnly bool
	RepackSmall        bool
	RepackUncompressed bool
}

var pruneOptions PruneOptions

func init() {
	cmdRoot.AddCommand(cmdPrune)
	f := cmdPrune.Flags()
	f.BoolVarP(&pruneOptions.DryRun, "dry-run", "n", false, "do not modify the repository, just print what would be done")
	f.StringVarP(&pruneOptions.UnsafeNoSpaceRecovery, "unsafe-recover-no-free-space", "", "", "UNSAFE, READ THE DOCUMENTATION BEFORE USING! Try to recover a repository stuck with no free space. Do not use without trying out 'prune --max-repack-size 0' first.")
	addPruneOptions(cmdPrune)
}

func addPruneOptions(c *cobra.Command) {
	f := c.Flags()
	f.StringVar(&pruneOptions.MaxUnused, "max-unused", "5%", "tolerate given `limit` of unused data (absolute value in bytes with suffixes k/K, m/M, g/G, t/T, a value in % or the word 'unlimited')")
	f.StringVar(&pruneOptions.MaxRepackSize, "max-repack-size", "", "maximum `size` to repack (allowed suffixes: k/K, m/M, g/G, t/T)")
	f.BoolVar(&pruneOptions.RepackCachableOnly, "repack-cacheable-only", false, "only repack packs which are cacheable")
	f.BoolVar(&pruneOptions.RepackSmall, "repack-small", false, "repack pack files below 80% of target pack size")
	f.BoolVar(&pruneOptions.RepackUncompressed, "repack-uncompressed", false, "repack all uncompressed data")
}

func verifyPruneOptions(opts *PruneOptions) error {
	opts.MaxRepackBytes = math.MaxUint64
	if len(opts.MaxRepackSize) > 0 {
		size, err := ui.ParseBytes(opts.MaxRepackSize)
		if err != nil {
			return err
		}
		opts.MaxRepackBytes = uint64(size)
	}
	if opts.UnsafeNoSpaceRecovery != "" {
		// prevent repacking data to make sure users cannot get stuck.
		opts.MaxRepackBytes = 0
	}

	maxUnused := strings.TrimSpace(opts.MaxUnused)
	if maxUnused == "" {
		return errors.Fatalf("invalid value for --max-unused: %q", opts.MaxUnused)
	}

	// parse MaxUnused either as unlimited, a percentage, or an absolute number of bytes
	switch {
	case maxUnused == "unlimited":
		opts.maxUnusedBytes = func(used uint64) uint64 {
			return math.MaxUint64
		}

	case strings.HasSuffix(maxUnused, "%"):
		maxUnused = strings.TrimSuffix(maxUnused, "%")
		p, err := strconv.ParseFloat(maxUnused, 64)
		if err != nil {
			return errors.Fatalf("invalid percentage %q passed for --max-unused: %v", opts.MaxUnused, err)
		}

		if p < 0 {
			return errors.Fatal("percentage for --max-unused must be positive")
		}

		if p >= 100 {
			return errors.Fatal("percentage for --max-unused must be below 100%")
		}

		opts.maxUnusedBytes = func(used uint64) uint64 {
			return uint64(p / (100 - p) * float64(used))
		}

	default:
		size, err := ui.ParseBytes(maxUnused)
		if err != nil {
			return errors.Fatalf("invalid number of bytes %q for --max-unused: %v", opts.MaxUnused, err)
		}

		opts.maxUnusedBytes = func(used uint64) uint64 {
			return uint64(size)
		}
	}

	return nil
}

func runPrune(ctx context.Context, opts PruneOptions, gopts GlobalOptions) error {
	err := verifyPruneOptions(&opts)
	if err != nil {
		return err
	}

	if opts.RepackUncompressed && gopts.Compression == repository.CompressionOff {
		return errors.Fatal("disabled compression and `--repack-uncompressed` are mutually exclusive")
	}

	repo, err := OpenRepository(ctx, gopts)
	if err != nil {
		return err
	}

	if repo.Backend().Connections() < 2 {
		return errors.Fatal("prune requires a backend connection limit of at least two")
	}

	if repo.Config().Version < 2 && opts.RepackUncompressed {
		return errors.Fatal("compression requires at least repository format version 2")
	}

	if opts.UnsafeNoSpaceRecovery != "" {
		repoID := repo.Config().ID
		if opts.UnsafeNoSpaceRecovery != repoID {
			return errors.Fatalf("must pass id '%s' to --unsafe-recover-no-free-space", repoID)
		}
		opts.unsafeRecovery = true
	}

	lock, ctx, err := lockRepoExclusive(ctx, repo, gopts.RetryLock, gopts.JSON)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	return runPruneWithRepo(ctx, opts, gopts, repo, restic.NewIDSet())
}

func runPruneWithRepo(ctx context.Context, opts PruneOptions, gopts GlobalOptions, repo *repository.Repository, ignoreSnapshots restic.IDSet) error {
	// we do not need index updates while pruning!
	repo.DisableAutoIndexUpdate()

	if repo.Cache == nil {
		Print("warning: running prune without a cache, this may be very slow!\n")
	}

	Verbosef("loading indexes...\n")
	// loading the index before the snapshots is ok, as we use an exclusive lock here
	err := repo.LoadIndex(ctx)
	if err != nil {
		return err
	}

	plan, stats, err := planPrune(ctx, opts, repo, ignoreSnapshots, gopts.Quiet)
	if err != nil {
		return err
	}

	if opts.DryRun {
		Verbosef("\nWould have made the following changes:")
	}

	err = printPruneStats(stats)
	if err != nil {
		return err
	}

	// Trigger GC to reset garbage collection threshold
	runtime.GC()

	return doPrune(ctx, opts, gopts, repo, plan)
}

type pruneStats struct {
	blobs struct {
		used      uint
		duplicate uint
		unused    uint
		remove    uint
		repack    uint
		repackrm  uint
	}
	size struct {
		used         uint64
		duplicate    uint64
		unused       uint64
		remove       uint64
		repack       uint64
		repackrm     uint64
		unref        uint64
		uncompressed uint64
	}
	packs struct {
		used       uint
		unused     uint
		partlyUsed uint
		unref      uint
		keep       uint
		repack     uint
		remove     uint
	}
}

type prunePlan struct {
	removePacksFirst restic.IDSet          // packs to remove first (unreferenced packs)
	repackPacks      restic.IDSet          // packs to repack
	keepBlobs        restic.CountedBlobSet // blobs to keep during repacking
	removePacks      restic.IDSet          // packs to remove
	ignorePacks      restic.IDSet          // packs to ignore when rebuilding the index
}

type packInfo struct {
	usedBlobs    uint
	unusedBlobs  uint
	usedSize     uint64
	unusedSize   uint64
	tpe          restic.BlobType
	uncompressed bool
}

type packInfoWithID struct {
	ID restic.ID
	packInfo
	mustCompress bool
}

// planPrune selects which files to rewrite and which to delete and which blobs to keep.
// Also some summary statistics are returned.
func planPrune(ctx context.Context, opts PruneOptions, repo restic.Repository, ignoreSnapshots restic.IDSet, quiet bool) (prunePlan, pruneStats, error) {
	var stats pruneStats

	usedBlobs, err := getUsedBlobs(ctx, repo, ignoreSnapshots, quiet)
	if err != nil {
		return prunePlan{}, stats, err
	}

	Verbosef("searching used packs...\n")
	keepBlobs, indexPack, err := packInfoFromIndex(ctx, repo.Index(), usedBlobs, &stats)
	if err != nil {
		return prunePlan{}, stats, err
	}

	Verbosef("collecting packs for deletion and repacking\n")
	plan, err := decidePackAction(ctx, opts, repo, indexPack, &stats, quiet)
	if err != nil {
		return prunePlan{}, stats, err
	}

	if len(plan.repackPacks) != 0 {
		blobCount := keepBlobs.Len()
		// when repacking, we do not want to keep blobs which are
		// already contained in kept packs, so delete them from keepBlobs
		repo.Index().Each(ctx, func(blob restic.PackedBlob) {
			if plan.removePacks.Has(blob.PackID) || plan.repackPacks.Has(blob.PackID) {
				return
			}
			keepBlobs.Delete(blob.BlobHandle)
		})

		if keepBlobs.Len() < blobCount/2 {
			// replace with copy to shrink map to necessary size if there's a chance to benefit
			keepBlobs = keepBlobs.Copy()
		}
	} else {
		// keepBlobs is only needed if packs are repacked
		keepBlobs = nil
	}
	plan.keepBlobs = keepBlobs

	return plan, stats, nil
}

func packInfoFromIndex(ctx context.Context, idx restic.MasterIndex, usedBlobs restic.CountedBlobSet, stats *pruneStats) (restic.CountedBlobSet, map[restic.ID]packInfo, error) {
	// iterate over all blobs in index to find out which blobs are duplicates
	// The counter in usedBlobs describes how many instances of the blob exist in the repository index
	// Thus 0 == blob is missing, 1 == blob exists once, >= 2 == duplicates exist
	idx.Each(ctx, func(blob restic.PackedBlob) {
		bh := blob.BlobHandle
		count, ok := usedBlobs[bh]
		if ok {
			if count < math.MaxUint8 {
				// don't overflow, but saturate count at 255
				// this can lead to a non-optimal pack selection, but won't cause
				// problems otherwise
				count++
			}

			usedBlobs[bh] = count
		}
	})

	// Check if all used blobs have been found in index
	missingBlobs := restic.NewBlobSet()
	for bh, count := range usedBlobs {
		if count == 0 {
			// blob does not exist in any pack files
			missingBlobs.Insert(bh)
		}
	}

	if len(missingBlobs) != 0 {
		Warnf("%v not found in the index\n\n"+
			"Integrity check failed: Data seems to be missing.\n"+
			"Will not start prune to prevent (additional) data loss!\n"+
			"Please report this error (along with the output of the 'prune' run) at\n"+
			"https://github.com/restic/restic/issues/new/choose\n", missingBlobs)
		return nil, nil, errorIndexIncomplete
	}

	indexPack := make(map[restic.ID]packInfo)

	// save computed pack header size
	for pid, hdrSize := range pack.Size(ctx, idx, true) {
		// initialize tpe with NumBlobTypes to indicate it's not set
		indexPack[pid] = packInfo{tpe: restic.NumBlobTypes, usedSize: uint64(hdrSize)}
	}

	hasDuplicates := false
	// iterate over all blobs in index to generate packInfo
	idx.Each(ctx, func(blob restic.PackedBlob) {
		ip := indexPack[blob.PackID]

		// Set blob type if not yet set
		if ip.tpe == restic.NumBlobTypes {
			ip.tpe = blob.Type
		}

		// mark mixed packs with "Invalid blob type"
		if ip.tpe != blob.Type {
			ip.tpe = restic.InvalidBlob
		}

		bh := blob.BlobHandle
		size := uint64(blob.Length)
		dupCount := usedBlobs[bh]
		switch {
		case dupCount >= 2:
			hasDuplicates = true
			// mark as unused for now, we will later on select one copy
			ip.unusedSize += size
			ip.unusedBlobs++

			// count as duplicate, will later on change one copy to be counted as used
			stats.size.duplicate += size
			stats.blobs.duplicate++
		case dupCount == 1: // used blob, not duplicate
			ip.usedSize += size
			ip.usedBlobs++

			stats.size.used += size
			stats.blobs.used++
		default: // unused blob
			ip.unusedSize += size
			ip.unusedBlobs++

			stats.size.unused += size
			stats.blobs.unused++
		}
		if !blob.IsCompressed() {
			ip.uncompressed = true
		}
		// update indexPack
		indexPack[blob.PackID] = ip
	})

	// if duplicate blobs exist, those will be set to either "used" or "unused":
	// - mark only one occurence of duplicate blobs as used
	// - if there are already some used blobs in a pack, possibly mark duplicates in this pack as "used"
	// - if there are no used blobs in a pack, possibly mark duplicates as "unused"
	if hasDuplicates {
		// iterate again over all blobs in index (this is pretty cheap, all in-mem)
		idx.Each(ctx, func(blob restic.PackedBlob) {
			bh := blob.BlobHandle
			count, ok := usedBlobs[bh]
			// skip non-duplicate, aka. normal blobs
			// count == 0 is used to mark that this was a duplicate blob with only a single occurence remaining
			if !ok || count == 1 {
				return
			}

			ip := indexPack[blob.PackID]
			size := uint64(blob.Length)
			switch {
			case ip.usedBlobs > 0, count == 0:
				// other used blobs in pack or "last" occurence ->  transition to used
				ip.usedSize += size
				ip.usedBlobs++
				ip.unusedSize -= size
				ip.unusedBlobs--
				// same for the global statistics
				stats.size.used += size
				stats.blobs.used++
				stats.size.duplicate -= size
				stats.blobs.duplicate--
				// let other occurences remain marked as unused
				usedBlobs[bh] = 1
			default:
				// remain unused and decrease counter
				count--
				if count == 1 {
					// setting count to 1 would lead to forgetting that this blob had duplicates
					// thus use the special value zero. This will select the last instance of the blob for keeping.
					count = 0
				}
				usedBlobs[bh] = count
			}
			// update indexPack
			indexPack[blob.PackID] = ip
		})
	}

	// Sanity check. If no duplicates exist, all blobs have value 1. After handling
	// duplicates, this also applies to duplicates.
	for _, count := range usedBlobs {
		if count != 1 {
			panic("internal error during blob selection")
		}
	}

	return usedBlobs, indexPack, nil
}

func decidePackAction(ctx context.Context, opts PruneOptions, repo restic.Repository, indexPack map[restic.ID]packInfo, stats *pruneStats, quiet bool) (prunePlan, error) {
	removePacksFirst := restic.NewIDSet()
	removePacks := restic.NewIDSet()
	repackPacks := restic.NewIDSet()

	var repackCandidates []packInfoWithID
	var repackSmallCandidates []packInfoWithID
	repoVersion := repo.Config().Version
	// only repack very small files by default
	targetPackSize := repo.PackSize() / 25
	if opts.RepackSmall {
		// consider files with at least 80% of the target size as large enough
		targetPackSize = repo.PackSize() / 5 * 4
	}

	// loop over all packs and decide what to do
	bar := newProgressMax(!quiet, uint64(len(indexPack)), "packs processed")
	err := repo.List(ctx, restic.PackFile, func(id restic.ID, packSize int64) error {
		p, ok := indexPack[id]
		if !ok {
			// Pack was not referenced in index and is not used  => immediately remove!
			Verboseff("will remove pack %v as it is unused and not indexed\n", id.Str())
			removePacksFirst.Insert(id)
			stats.size.unref += uint64(packSize)
			return nil
		}

		if p.unusedSize+p.usedSize != uint64(packSize) && p.usedBlobs != 0 {
			// Pack size does not fit and pack is needed => error
			// If the pack is not needed, this is no error, the pack can
			// and will be simply removed, see below.
			Warnf("pack %s: calculated size %d does not match real size %d\nRun 'restic repair index'.\n",
				id.Str(), p.unusedSize+p.usedSize, packSize)
			return errorSizeNotMatching
		}

		// statistics
		switch {
		case p.usedBlobs == 0:
			stats.packs.unused++
		case p.unusedBlobs == 0:
			stats.packs.used++
		default:
			stats.packs.partlyUsed++
		}

		if p.uncompressed {
			stats.size.uncompressed += p.unusedSize + p.usedSize
		}
		mustCompress := false
		if repoVersion >= 2 {
			// repo v2: always repack tree blobs if uncompressed
			// compress data blobs if requested
			mustCompress = (p.tpe == restic.TreeBlob || opts.RepackUncompressed) && p.uncompressed
		}

		// decide what to do
		switch {
		case p.usedBlobs == 0:
			// All blobs in pack are no longer used => remove pack!
			removePacks.Insert(id)
			stats.blobs.remove += p.unusedBlobs
			stats.size.remove += p.unusedSize

		case opts.RepackCachableOnly && p.tpe == restic.DataBlob:
			// if this is a data pack and --repack-cacheable-only is set => keep pack!
			stats.packs.keep++

		case p.unusedBlobs == 0 && p.tpe != restic.InvalidBlob && !mustCompress:
			if packSize >= int64(targetPackSize) {
				// All blobs in pack are used and not mixed => keep pack!
				stats.packs.keep++
			} else {
				repackSmallCandidates = append(repackSmallCandidates, packInfoWithID{ID: id, packInfo: p, mustCompress: mustCompress})
			}

		default:
			// all other packs are candidates for repacking
			repackCandidates = append(repackCandidates, packInfoWithID{ID: id, packInfo: p, mustCompress: mustCompress})
		}

		delete(indexPack, id)
		bar.Add(1)
		return nil
	})
	bar.Done()
	if err != nil {
		return prunePlan{}, err
	}

	// At this point indexPacks contains only missing packs!

	// missing packs that are not needed can be ignored
	ignorePacks := restic.NewIDSet()
	for id, p := range indexPack {
		if p.usedBlobs == 0 {
			ignorePacks.Insert(id)
			stats.blobs.remove += p.unusedBlobs
			stats.size.remove += p.unusedSize
			delete(indexPack, id)
		}
	}

	if len(indexPack) != 0 {
		Warnf("The index references %d needed pack files which are missing from the repository:\n", len(indexPack))
		for id := range indexPack {
			Warnf("  %v\n", id)
		}
		return prunePlan{}, errorPacksMissing
	}
	if len(ignorePacks) != 0 {
		Warnf("Missing but unneeded pack files are referenced in the index, will be repaired\n")
		for id := range ignorePacks {
			Warnf("will forget missing pack file %v\n", id)
		}
	}

	if len(repackSmallCandidates) < 10 {
		// too few small files to be worth the trouble, this also prevents endlessly repacking
		// if there is just a single pack file below the target size
		stats.packs.keep += uint(len(repackSmallCandidates))
	} else {
		repackCandidates = append(repackCandidates, repackSmallCandidates...)
	}

	// Sort repackCandidates such that packs with highest ratio unused/used space are picked first.
	// This is equivalent to sorting by unused / total space.
	// Instead of unused[i] / used[i] > unused[j] / used[j] we use
	// unused[i] * used[j] > unused[j] * used[i] as uint32*uint32 < uint64
	// Moreover packs containing trees and too small packs are sorted to the beginning
	sort.Slice(repackCandidates, func(i, j int) bool {
		pi := repackCandidates[i].packInfo
		pj := repackCandidates[j].packInfo
		switch {
		case pi.tpe != restic.DataBlob && pj.tpe == restic.DataBlob:
			return true
		case pj.tpe != restic.DataBlob && pi.tpe == restic.DataBlob:
			return false
		case pi.unusedSize+pi.usedSize < uint64(targetPackSize) && pj.unusedSize+pj.usedSize >= uint64(targetPackSize):
			return true
		case pj.unusedSize+pj.usedSize < uint64(targetPackSize) && pi.unusedSize+pi.usedSize >= uint64(targetPackSize):
			return false
		}
		return pi.unusedSize*pj.usedSize > pj.unusedSize*pi.usedSize
	})

	repack := func(id restic.ID, p packInfo) {
		repackPacks.Insert(id)
		stats.blobs.repack += p.unusedBlobs + p.usedBlobs
		stats.size.repack += p.unusedSize + p.usedSize
		stats.blobs.repackrm += p.unusedBlobs
		stats.size.repackrm += p.unusedSize
		if p.uncompressed {
			stats.size.uncompressed -= p.unusedSize + p.usedSize
		}
	}

	// calculate limit for number of unused bytes in the repo after repacking
	maxUnusedSizeAfter := opts.maxUnusedBytes(stats.size.used)

	for _, p := range repackCandidates {
		reachedUnusedSizeAfter := (stats.size.unused-stats.size.remove-stats.size.repackrm < maxUnusedSizeAfter)
		reachedRepackSize := stats.size.repack+p.unusedSize+p.usedSize >= opts.MaxRepackBytes
		packIsLargeEnough := p.unusedSize+p.usedSize >= uint64(targetPackSize)

		switch {
		case reachedRepackSize:
			stats.packs.keep++

		case p.tpe != restic.DataBlob, p.mustCompress:
			// repacking non-data packs / uncompressed-trees is only limited by repackSize
			repack(p.ID, p.packInfo)

		case reachedUnusedSizeAfter && packIsLargeEnough:
			// for all other packs stop repacking if tolerated unused size is reached.
			stats.packs.keep++

		default:
			repack(p.ID, p.packInfo)
		}
	}

	stats.packs.unref = uint(len(removePacksFirst))
	stats.packs.repack = uint(len(repackPacks))
	stats.packs.remove = uint(len(removePacks))

	if repo.Config().Version < 2 {
		// compression not supported for repository format version 1
		stats.size.uncompressed = 0
	}

	return prunePlan{removePacksFirst: removePacksFirst,
		removePacks: removePacks,
		repackPacks: repackPacks,
		ignorePacks: ignorePacks,
	}, nil
}

// printPruneStats prints out the statistics
func printPruneStats(stats pruneStats) error {
	Verboseff("\nused:         %10d blobs / %s\n", stats.blobs.used, ui.FormatBytes(stats.size.used))
	if stats.blobs.duplicate > 0 {
		Verboseff("duplicates:   %10d blobs / %s\n", stats.blobs.duplicate, ui.FormatBytes(stats.size.duplicate))
	}
	Verboseff("unused:       %10d blobs / %s\n", stats.blobs.unused, ui.FormatBytes(stats.size.unused))
	if stats.size.unref > 0 {
		Verboseff("unreferenced:                    %s\n", ui.FormatBytes(stats.size.unref))
	}
	totalBlobs := stats.blobs.used + stats.blobs.unused + stats.blobs.duplicate
	totalSize := stats.size.used + stats.size.duplicate + stats.size.unused + stats.size.unref
	unusedSize := stats.size.duplicate + stats.size.unused
	Verboseff("total:        %10d blobs / %s\n", totalBlobs, ui.FormatBytes(totalSize))
	Verboseff("unused size: %s of total size\n", ui.FormatPercent(unusedSize, totalSize))

	Verbosef("\nto repack:    %10d blobs / %s\n", stats.blobs.repack, ui.FormatBytes(stats.size.repack))
	Verbosef("this removes: %10d blobs / %s\n", stats.blobs.repackrm, ui.FormatBytes(stats.size.repackrm))
	Verbosef("to delete:    %10d blobs / %s\n", stats.blobs.remove, ui.FormatBytes(stats.size.remove+stats.size.unref))
	totalPruneSize := stats.size.remove + stats.size.repackrm + stats.size.unref
	Verbosef("total prune:  %10d blobs / %s\n", stats.blobs.remove+stats.blobs.repackrm, ui.FormatBytes(totalPruneSize))
	if stats.size.uncompressed > 0 {
		Verbosef("not yet compressed:              %s\n", ui.FormatBytes(stats.size.uncompressed))
	}
	Verbosef("remaining:    %10d blobs / %s\n", totalBlobs-(stats.blobs.remove+stats.blobs.repackrm), ui.FormatBytes(totalSize-totalPruneSize))
	unusedAfter := unusedSize - stats.size.remove - stats.size.repackrm
	Verbosef("unused size after prune: %s (%s of remaining size)\n",
		ui.FormatBytes(unusedAfter), ui.FormatPercent(unusedAfter, totalSize-totalPruneSize))
	Verbosef("\n")
	Verboseff("totally used packs: %10d\n", stats.packs.used)
	Verboseff("partly used packs:  %10d\n", stats.packs.partlyUsed)
	Verboseff("unused packs:       %10d\n\n", stats.packs.unused)

	Verboseff("to keep:      %10d packs\n", stats.packs.keep)
	Verboseff("to repack:    %10d packs\n", stats.packs.repack)
	Verboseff("to delete:    %10d packs\n", stats.packs.remove)
	if stats.packs.unref > 0 {
		Verboseff("to delete:    %10d unreferenced packs\n\n", stats.packs.unref)
	}
	return nil
}

// doPrune does the actual pruning:
// - remove unreferenced packs first
// - repack given pack files while keeping the given blobs
// - rebuild the index while ignoring all files that will be deleted
// - delete the files
// plan.removePacks and plan.ignorePacks are modified in this function.
func doPrune(ctx context.Context, opts PruneOptions, gopts GlobalOptions, repo restic.Repository, plan prunePlan) (err error) {
	if opts.DryRun {
		if !gopts.JSON && gopts.verbosity >= 2 {
			Printf("Repeated prune dry-runs can report slightly different amounts of data to keep or repack. This is expected behavior.\n\n")
			if len(plan.removePacksFirst) > 0 {
				Printf("Would have removed the following unreferenced packs:\n%v\n\n", plan.removePacksFirst)
			}
			Printf("Would have repacked and removed the following packs:\n%v\n\n", plan.repackPacks)
			Printf("Would have removed the following no longer used packs:\n%v\n\n", plan.removePacks)
		}
		// Always quit here if DryRun was set!
		return nil
	}

	// unreferenced packs can be safely deleted first
	if len(plan.removePacksFirst) != 0 {
		Verbosef("deleting unreferenced packs\n")
		DeleteFiles(ctx, gopts, repo, plan.removePacksFirst, restic.PackFile)
	}

	if len(plan.repackPacks) != 0 {
		Verbosef("repacking packs\n")
		bar := newProgressMax(!gopts.Quiet, uint64(len(plan.repackPacks)), "packs repacked")
		_, err := repository.Repack(ctx, repo, repo, plan.repackPacks, plan.keepBlobs, bar)
		bar.Done()
		if err != nil {
			return errors.Fatal(err.Error())
		}

		// Also remove repacked packs
		plan.removePacks.Merge(plan.repackPacks)

		if len(plan.keepBlobs) != 0 {
			Warnf("%v was not repacked\n\n"+
				"Integrity check failed.\n"+
				"Please report this error (along with the output of the 'prune' run) at\n"+
				"https://github.com/restic/restic/issues/new/choose\n", plan.keepBlobs)
			return errors.Fatal("internal error: blobs were not repacked")
		}

		// allow GC of the blob set
		plan.keepBlobs = nil
	}

	if len(plan.ignorePacks) == 0 {
		plan.ignorePacks = plan.removePacks
	} else {
		plan.ignorePacks.Merge(plan.removePacks)
	}

	if opts.unsafeRecovery {
		Verbosef("deleting index files\n")
		indexFiles := repo.Index().(*index.MasterIndex).IDs()
		err = DeleteFilesChecked(ctx, gopts, repo, indexFiles, restic.IndexFile)
		if err != nil {
			return errors.Fatalf("%s", err)
		}
	} else if len(plan.ignorePacks) != 0 {
		err = rebuildIndexFiles(ctx, gopts, repo, plan.ignorePacks, nil)
		if err != nil {
			return errors.Fatalf("%s", err)
		}
	}

	if len(plan.removePacks) != 0 {
		Verbosef("removing %d old packs\n", len(plan.removePacks))
		DeleteFiles(ctx, gopts, repo, plan.removePacks, restic.PackFile)
	}

	if opts.unsafeRecovery {
		_, err = writeIndexFiles(ctx, gopts, repo, plan.ignorePacks, nil)
		if err != nil {
			return errors.Fatalf("%s", err)
		}
	}

	Verbosef("done\n")
	return nil
}

func writeIndexFiles(ctx context.Context, gopts GlobalOptions, repo restic.Repository, removePacks restic.IDSet, extraObsolete restic.IDs) (restic.IDSet, error) {
	Verbosef("rebuilding index\n")

	bar := newProgressMax(!gopts.Quiet, 0, "packs processed")
	obsoleteIndexes, err := repo.Index().Save(ctx, repo, removePacks, extraObsolete, bar)
	bar.Done()
	return obsoleteIndexes, err
}

func rebuildIndexFiles(ctx context.Context, gopts GlobalOptions, repo restic.Repository, removePacks restic.IDSet, extraObsolete restic.IDs) error {
	obsoleteIndexes, err := writeIndexFiles(ctx, gopts, repo, removePacks, extraObsolete)
	if err != nil {
		return err
	}

	Verbosef("deleting obsolete index files\n")
	return DeleteFilesChecked(ctx, gopts, repo, obsoleteIndexes, restic.IndexFile)
}

func getUsedBlobs(ctx context.Context, repo restic.Repository, ignoreSnapshots restic.IDSet, quiet bool) (usedBlobs restic.CountedBlobSet, err error) {
	var snapshotTrees restic.IDs
	Verbosef("loading all snapshots...\n")
	err = restic.ForAllSnapshots(ctx, repo.Backend(), repo, ignoreSnapshots,
		func(id restic.ID, sn *restic.Snapshot, err error) error {
			if err != nil {
				debug.Log("failed to load snapshot %v (error %v)", id, err)
				return err
			}
			debug.Log("add snapshot %v (tree %v)", id, *sn.Tree)
			snapshotTrees = append(snapshotTrees, *sn.Tree)
			return nil
		})
	if err != nil {
		return nil, errors.Fatalf("failed loading snapshot: %v", err)
	}

	Verbosef("finding data that is still in use for %d snapshots\n", len(snapshotTrees))

	usedBlobs = restic.NewCountedBlobSet()

	bar := newProgressMax(!quiet, uint64(len(snapshotTrees)), "snapshots")
	defer bar.Done()

	err = restic.FindUsedBlobs(ctx, repo, snapshotTrees, usedBlobs, bar)
	if err != nil {
		if repo.Backend().IsNotExist(err) {
			return nil, errors.Fatal("unable to load a tree from the repository: " + err.Error())
		}

		return nil, err
	}
	return usedBlobs, nil
}
