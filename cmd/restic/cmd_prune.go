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
	"github.com/restic/restic/internal/ui/progress"
	"github.com/restic/restic/internal/ui/termstatus"

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
	RunE: func(cmd *cobra.Command, _ []string) error {
		term, cancel := setupTermstatus()
		defer cancel()
		return runPrune(cmd.Context(), pruneOptions, globalOptions, term)
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
	addPruneOptions(cmdPrune, &pruneOptions)
}

func addPruneOptions(c *cobra.Command, pruneOptions *PruneOptions) {
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
		opts.maxUnusedBytes = func(_ uint64) uint64 {
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

		opts.maxUnusedBytes = func(_ uint64) uint64 {
			return uint64(size)
		}
	}

	return nil
}

func runPrune(ctx context.Context, opts PruneOptions, gopts GlobalOptions, term *termstatus.Terminal) error {
	err := verifyPruneOptions(&opts)
	if err != nil {
		return err
	}

	if opts.RepackUncompressed && gopts.Compression == repository.CompressionOff {
		return errors.Fatal("disabled compression and `--repack-uncompressed` are mutually exclusive")
	}

	ctx, repo, unlock, err := openWithExclusiveLock(ctx, gopts, false)
	if err != nil {
		return err
	}
	defer unlock()

	if repo.Connections() < 2 {
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

	return runPruneWithRepo(ctx, opts, gopts, repo, restic.NewIDSet(), term)
}

func runPruneWithRepo(ctx context.Context, opts PruneOptions, gopts GlobalOptions, repo *repository.Repository, ignoreSnapshots restic.IDSet, term *termstatus.Terminal) error {
	// we do not need index updates while pruning!
	repo.DisableAutoIndexUpdate()

	if repo.Cache == nil {
		Print("warning: running prune without a cache, this may be very slow!\n")
	}

	printer := newTerminalProgressPrinter(gopts.verbosity, term)

	printer.P("loading indexes...\n")
	// loading the index before the snapshots is ok, as we use an exclusive lock here
	bar := newIndexTerminalProgress(gopts.Quiet, gopts.JSON, term)
	err := repo.LoadIndex(ctx, bar)
	if err != nil {
		return err
	}

	plan, stats, err := PlanPrune(ctx, opts, repo, func(ctx context.Context, repo restic.Repository) (usedBlobs restic.CountedBlobSet, err error) {
		return getUsedBlobs(ctx, repo, ignoreSnapshots, printer)
	}, printer)
	if err != nil {
		return err
	}

	if opts.DryRun {
		printer.P("\nWould have made the following changes:")
	}

	err = printPruneStats(printer, stats)
	if err != nil {
		return err
	}

	// Trigger GC to reset garbage collection threshold
	runtime.GC()

	return DoPrune(ctx, opts, repo, plan, printer)
}

type PruneStats struct {
	Blobs struct {
		Used      uint
		Duplicate uint
		Unused    uint
		Remove    uint
		Repack    uint
		Repackrm  uint
	}
	Size struct {
		Used         uint64
		Duplicate    uint64
		Unused       uint64
		Remove       uint64
		Repack       uint64
		Repackrm     uint64
		Unref        uint64
		Uncompressed uint64
	}
	Packs struct {
		Used       uint
		Unused     uint
		PartlyUsed uint
		Unref      uint
		Keep       uint
		Repack     uint
		Remove     uint
	}
}

type PrunePlan struct {
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

// PlanPrune selects which files to rewrite and which to delete and which blobs to keep.
// Also some summary statistics are returned.
func PlanPrune(ctx context.Context, opts PruneOptions, repo restic.Repository, getUsedBlobs func(ctx context.Context, repo restic.Repository) (usedBlobs restic.CountedBlobSet, err error), printer progress.Printer) (PrunePlan, PruneStats, error) {
	var stats PruneStats

	usedBlobs, err := getUsedBlobs(ctx, repo)
	if err != nil {
		return PrunePlan{}, stats, err
	}

	printer.P("searching used packs...\n")
	keepBlobs, indexPack, err := packInfoFromIndex(ctx, repo.Index(), usedBlobs, &stats, printer)
	if err != nil {
		return PrunePlan{}, stats, err
	}

	printer.P("collecting packs for deletion and repacking\n")
	plan, err := decidePackAction(ctx, opts, repo, indexPack, &stats, printer)
	if err != nil {
		return PrunePlan{}, stats, err
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

func packInfoFromIndex(ctx context.Context, idx restic.MasterIndex, usedBlobs restic.CountedBlobSet, stats *PruneStats, printer progress.Printer) (restic.CountedBlobSet, map[restic.ID]packInfo, error) {
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
		printer.E("%v not found in the index\n\n"+
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
			stats.Size.Duplicate += size
			stats.Blobs.Duplicate++
		case dupCount == 1: // used blob, not duplicate
			ip.usedSize += size
			ip.usedBlobs++

			stats.Size.Used += size
			stats.Blobs.Used++
		default: // unused blob
			ip.unusedSize += size
			ip.unusedBlobs++

			stats.Size.Unused += size
			stats.Blobs.Unused++
		}
		if !blob.IsCompressed() {
			ip.uncompressed = true
		}
		// update indexPack
		indexPack[blob.PackID] = ip
	})

	// if duplicate blobs exist, those will be set to either "used" or "unused":
	// - mark only one occurrence of duplicate blobs as used
	// - if there are already some used blobs in a pack, possibly mark duplicates in this pack as "used"
	// - if there are no used blobs in a pack, possibly mark duplicates as "unused"
	if hasDuplicates {
		// iterate again over all blobs in index (this is pretty cheap, all in-mem)
		idx.Each(ctx, func(blob restic.PackedBlob) {
			bh := blob.BlobHandle
			count, ok := usedBlobs[bh]
			// skip non-duplicate, aka. normal blobs
			// count == 0 is used to mark that this was a duplicate blob with only a single occurrence remaining
			if !ok || count == 1 {
				return
			}

			ip := indexPack[blob.PackID]
			size := uint64(blob.Length)
			switch {
			case ip.usedBlobs > 0, count == 0:
				// other used blobs in pack or "last" occurrence ->  transition to used
				ip.usedSize += size
				ip.usedBlobs++
				ip.unusedSize -= size
				ip.unusedBlobs--
				// same for the global statistics
				stats.Size.Used += size
				stats.Blobs.Used++
				stats.Size.Duplicate -= size
				stats.Blobs.Duplicate--
				// let other occurrences remain marked as unused
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

func decidePackAction(ctx context.Context, opts PruneOptions, repo restic.Repository, indexPack map[restic.ID]packInfo, stats *PruneStats, printer progress.Printer) (PrunePlan, error) {
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
	bar := printer.NewCounter("packs processed")
	bar.SetMax(uint64(len(indexPack)))
	err := repo.List(ctx, restic.PackFile, func(id restic.ID, packSize int64) error {
		p, ok := indexPack[id]
		if !ok {
			// Pack was not referenced in index and is not used  => immediately remove!
			printer.V("will remove pack %v as it is unused and not indexed\n", id.Str())
			removePacksFirst.Insert(id)
			stats.Size.Unref += uint64(packSize)
			return nil
		}

		if p.unusedSize+p.usedSize != uint64(packSize) && p.usedBlobs != 0 {
			// Pack size does not fit and pack is needed => error
			// If the pack is not needed, this is no error, the pack can
			// and will be simply removed, see below.
			printer.E("pack %s: calculated size %d does not match real size %d\nRun 'restic repair index'.\n",
				id.Str(), p.unusedSize+p.usedSize, packSize)
			return errorSizeNotMatching
		}

		// statistics
		switch {
		case p.usedBlobs == 0:
			stats.Packs.Unused++
		case p.unusedBlobs == 0:
			stats.Packs.Used++
		default:
			stats.Packs.PartlyUsed++
		}

		if p.uncompressed {
			stats.Size.Uncompressed += p.unusedSize + p.usedSize
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
			stats.Blobs.Remove += p.unusedBlobs
			stats.Size.Remove += p.unusedSize

		case opts.RepackCachableOnly && p.tpe == restic.DataBlob:
			// if this is a data pack and --repack-cacheable-only is set => keep pack!
			stats.Packs.Keep++

		case p.unusedBlobs == 0 && p.tpe != restic.InvalidBlob && !mustCompress:
			if packSize >= int64(targetPackSize) {
				// All blobs in pack are used and not mixed => keep pack!
				stats.Packs.Keep++
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
		return PrunePlan{}, err
	}

	// At this point indexPacks contains only missing packs!

	// missing packs that are not needed can be ignored
	ignorePacks := restic.NewIDSet()
	for id, p := range indexPack {
		if p.usedBlobs == 0 {
			ignorePacks.Insert(id)
			stats.Blobs.Remove += p.unusedBlobs
			stats.Size.Remove += p.unusedSize
			delete(indexPack, id)
		}
	}

	if len(indexPack) != 0 {
		printer.E("The index references %d needed pack files which are missing from the repository:\n", len(indexPack))
		for id := range indexPack {
			printer.E("  %v\n", id)
		}
		return PrunePlan{}, errorPacksMissing
	}
	if len(ignorePacks) != 0 {
		printer.E("Missing but unneeded pack files are referenced in the index, will be repaired\n")
		for id := range ignorePacks {
			printer.E("will forget missing pack file %v\n", id)
		}
	}

	if len(repackSmallCandidates) < 10 {
		// too few small files to be worth the trouble, this also prevents endlessly repacking
		// if there is just a single pack file below the target size
		stats.Packs.Keep += uint(len(repackSmallCandidates))
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
		stats.Blobs.Repack += p.unusedBlobs + p.usedBlobs
		stats.Size.Repack += p.unusedSize + p.usedSize
		stats.Blobs.Repackrm += p.unusedBlobs
		stats.Size.Repackrm += p.unusedSize
		if p.uncompressed {
			stats.Size.Uncompressed -= p.unusedSize + p.usedSize
		}
	}

	// calculate limit for number of unused bytes in the repo after repacking
	maxUnusedSizeAfter := opts.maxUnusedBytes(stats.Size.Used)

	for _, p := range repackCandidates {
		reachedUnusedSizeAfter := (stats.Size.Unused-stats.Size.Remove-stats.Size.Repackrm < maxUnusedSizeAfter)
		reachedRepackSize := stats.Size.Repack+p.unusedSize+p.usedSize >= opts.MaxRepackBytes
		packIsLargeEnough := p.unusedSize+p.usedSize >= uint64(targetPackSize)

		switch {
		case reachedRepackSize:
			stats.Packs.Keep++

		case p.tpe != restic.DataBlob, p.mustCompress:
			// repacking non-data packs / uncompressed-trees is only limited by repackSize
			repack(p.ID, p.packInfo)

		case reachedUnusedSizeAfter && packIsLargeEnough:
			// for all other packs stop repacking if tolerated unused size is reached.
			stats.Packs.Keep++

		default:
			repack(p.ID, p.packInfo)
		}
	}

	stats.Packs.Unref = uint(len(removePacksFirst))
	stats.Packs.Repack = uint(len(repackPacks))
	stats.Packs.Remove = uint(len(removePacks))

	if repo.Config().Version < 2 {
		// compression not supported for repository format version 1
		stats.Size.Uncompressed = 0
	}

	return PrunePlan{removePacksFirst: removePacksFirst,
		removePacks: removePacks,
		repackPacks: repackPacks,
		ignorePacks: ignorePacks,
	}, nil
}

// printPruneStats prints out the statistics
func printPruneStats(printer progress.Printer, stats PruneStats) error {
	printer.V("\nused:         %10d blobs / %s\n", stats.Blobs.Used, ui.FormatBytes(stats.Size.Used))
	if stats.Blobs.Duplicate > 0 {
		printer.V("duplicates:   %10d blobs / %s\n", stats.Blobs.Duplicate, ui.FormatBytes(stats.Size.Duplicate))
	}
	printer.V("unused:       %10d blobs / %s\n", stats.Blobs.Unused, ui.FormatBytes(stats.Size.Unused))
	if stats.Size.Unref > 0 {
		printer.V("unreferenced:                    %s\n", ui.FormatBytes(stats.Size.Unref))
	}
	totalBlobs := stats.Blobs.Used + stats.Blobs.Unused + stats.Blobs.Duplicate
	totalSize := stats.Size.Used + stats.Size.Duplicate + stats.Size.Unused + stats.Size.Unref
	unusedSize := stats.Size.Duplicate + stats.Size.Unused
	printer.V("total:        %10d blobs / %s\n", totalBlobs, ui.FormatBytes(totalSize))
	printer.V("unused size: %s of total size\n", ui.FormatPercent(unusedSize, totalSize))

	printer.P("\nto repack:    %10d blobs / %s\n", stats.Blobs.Repack, ui.FormatBytes(stats.Size.Repack))
	printer.P("this removes: %10d blobs / %s\n", stats.Blobs.Repackrm, ui.FormatBytes(stats.Size.Repackrm))
	printer.P("to delete:    %10d blobs / %s\n", stats.Blobs.Remove, ui.FormatBytes(stats.Size.Remove+stats.Size.Unref))
	totalPruneSize := stats.Size.Remove + stats.Size.Repackrm + stats.Size.Unref
	printer.P("total prune:  %10d blobs / %s\n", stats.Blobs.Remove+stats.Blobs.Repackrm, ui.FormatBytes(totalPruneSize))
	if stats.Size.Uncompressed > 0 {
		printer.P("not yet compressed:              %s\n", ui.FormatBytes(stats.Size.Uncompressed))
	}
	printer.P("remaining:    %10d blobs / %s\n", totalBlobs-(stats.Blobs.Remove+stats.Blobs.Repackrm), ui.FormatBytes(totalSize-totalPruneSize))
	unusedAfter := unusedSize - stats.Size.Remove - stats.Size.Repackrm
	printer.P("unused size after prune: %s (%s of remaining size)\n",
		ui.FormatBytes(unusedAfter), ui.FormatPercent(unusedAfter, totalSize-totalPruneSize))
	printer.P("\n")
	printer.V("totally used packs: %10d\n", stats.Packs.Used)
	printer.V("partly used packs:  %10d\n", stats.Packs.PartlyUsed)
	printer.V("unused packs:       %10d\n\n", stats.Packs.Unused)

	printer.V("to keep:      %10d packs\n", stats.Packs.Keep)
	printer.V("to repack:    %10d packs\n", stats.Packs.Repack)
	printer.V("to delete:    %10d packs\n", stats.Packs.Remove)
	if stats.Packs.Unref > 0 {
		printer.V("to delete:    %10d unreferenced packs\n\n", stats.Packs.Unref)
	}
	return nil
}

// DoPrune does the actual pruning:
// - remove unreferenced packs first
// - repack given pack files while keeping the given blobs
// - rebuild the index while ignoring all files that will be deleted
// - delete the files
// plan.removePacks and plan.ignorePacks are modified in this function.
func DoPrune(ctx context.Context, opts PruneOptions, repo restic.Repository, plan PrunePlan, printer progress.Printer) (err error) {
	if opts.DryRun {
		printer.V("Repeated prune dry-runs can report slightly different amounts of data to keep or repack. This is expected behavior.\n\n")
		if len(plan.removePacksFirst) > 0 {
			printer.V("Would have removed the following unreferenced packs:\n%v\n\n", plan.removePacksFirst)
		}
		printer.V("Would have repacked and removed the following packs:\n%v\n\n", plan.repackPacks)
		printer.V("Would have removed the following no longer used packs:\n%v\n\n", plan.removePacks)
		// Always quit here if DryRun was set!
		return nil
	}

	// unreferenced packs can be safely deleted first
	if len(plan.removePacksFirst) != 0 {
		printer.P("deleting unreferenced packs\n")
		DeleteFiles(ctx, repo, plan.removePacksFirst, restic.PackFile, printer)
	}

	if len(plan.repackPacks) != 0 {
		printer.P("repacking packs\n")
		bar := printer.NewCounter("packs repacked")
		bar.SetMax(uint64(len(plan.repackPacks)))
		_, err := repository.Repack(ctx, repo, repo, plan.repackPacks, plan.keepBlobs, bar)
		bar.Done()
		if err != nil {
			return errors.Fatal(err.Error())
		}

		// Also remove repacked packs
		plan.removePacks.Merge(plan.repackPacks)

		if len(plan.keepBlobs) != 0 {
			printer.E("%v was not repacked\n\n"+
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
		printer.P("deleting index files\n")
		indexFiles := repo.Index().(*index.MasterIndex).IDs()
		err = DeleteFilesChecked(ctx, repo, indexFiles, restic.IndexFile, printer)
		if err != nil {
			return errors.Fatalf("%s", err)
		}
	} else if len(plan.ignorePacks) != 0 {
		err = rebuildIndexFiles(ctx, repo, plan.ignorePacks, nil, false, printer)
		if err != nil {
			return errors.Fatalf("%s", err)
		}
	}

	if len(plan.removePacks) != 0 {
		printer.P("removing %d old packs\n", len(plan.removePacks))
		DeleteFiles(ctx, repo, plan.removePacks, restic.PackFile, printer)
	}

	if opts.unsafeRecovery {
		err = rebuildIndexFiles(ctx, repo, plan.ignorePacks, nil, true, printer)
		if err != nil {
			return errors.Fatalf("%s", err)
		}
	}

	printer.P("done\n")
	return nil
}

func rebuildIndexFiles(ctx context.Context, repo restic.Repository, removePacks restic.IDSet, extraObsolete restic.IDs, skipDeletion bool, printer progress.Printer) error {
	printer.P("rebuilding index\n")

	bar := printer.NewCounter("packs processed")
	return repo.Index().Save(ctx, repo, removePacks, extraObsolete, restic.MasterIndexSaveOpts{
		SaveProgress: bar,
		DeleteProgress: func() *progress.Counter {
			return printer.NewCounter("old indexes deleted")
		},
		DeleteReport: func(id restic.ID, _ error) {
			printer.VV("removed index %v\n", id.String())
		},
		SkipDeletion: skipDeletion,
	})
}

func getUsedBlobs(ctx context.Context, repo restic.Repository, ignoreSnapshots restic.IDSet, printer progress.Printer) (usedBlobs restic.CountedBlobSet, err error) {
	var snapshotTrees restic.IDs
	printer.P("loading all snapshots...\n")
	err = restic.ForAllSnapshots(ctx, repo, repo, ignoreSnapshots,
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

	printer.P("finding data that is still in use for %d snapshots\n", len(snapshotTrees))

	usedBlobs = restic.NewCountedBlobSet()

	bar := printer.NewCounter("snapshots")
	bar.SetMax(uint64(len(snapshotTrees)))
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
