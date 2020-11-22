package main

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"

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
		return runPrune(pruneOptions, globalOptions)
	},
}

// PruneOptions collects all options for the cleanup command.
type PruneOptions struct {
	DryRun bool

	MaxUnused      string
	maxUnusedBytes func(used uint64) (unused uint64) // calculates the number of unused bytes after repacking, according to MaxUnused

	MaxRepackSize  string
	MaxRepackBytes uint64

	RepackCachableOnly bool
}

var pruneOptions PruneOptions

func init() {
	cmdRoot.AddCommand(cmdPrune)
	f := cmdPrune.Flags()
	f.BoolVarP(&pruneOptions.DryRun, "dry-run", "n", false, "do not modify the repository, just print what would be done")
	addPruneOptions(cmdPrune)
}

func addPruneOptions(c *cobra.Command) {
	f := c.Flags()
	f.StringVar(&pruneOptions.MaxUnused, "max-unused", "5%", "tolerate given `limit` of unused data (absolute value in bytes with suffixes k/K, m/M, g/G, t/T, a value in % or the word 'unlimited')")
	f.StringVar(&pruneOptions.MaxRepackSize, "max-repack-size", "", "maximum `size` to repack (allowed suffixes: k/K, m/M, g/G, t/T)")
	f.BoolVar(&pruneOptions.RepackCachableOnly, "repack-cacheable-only", false, "only repack packs which are cacheable")
}

func verifyPruneOptions(opts *PruneOptions) error {
	if len(opts.MaxRepackSize) > 0 {
		size, err := parseSizeStr(opts.MaxRepackSize)
		if err != nil {
			return err
		}
		opts.MaxRepackBytes = uint64(size)
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
		size, err := parseSizeStr(maxUnused)
		if err != nil {
			return errors.Fatalf("invalid number of bytes %q for --max-unused: %v", opts.MaxUnused, err)
		}

		opts.maxUnusedBytes = func(used uint64) uint64 {
			return uint64(size)
		}
	}

	return nil
}

func shortenStatus(maxLength int, s string) string {
	if len(s) <= maxLength {
		return s
	}

	if maxLength < 3 {
		return s[:maxLength]
	}

	return s[:maxLength-3] + "..."
}

func runPrune(opts PruneOptions, gopts GlobalOptions) error {
	err := verifyPruneOptions(&opts)
	if err != nil {
		return err
	}

	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	lock, err := lockRepoExclusive(gopts.ctx, repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	return runPruneWithRepo(opts, gopts, repo, restic.NewIDSet())
}

func runPruneWithRepo(opts PruneOptions, gopts GlobalOptions, repo *repository.Repository, ignoreSnapshots restic.IDSet) error {
	// we do not need index updates while pruning!
	repo.DisableAutoIndexUpdate()

	if repo.Cache == nil {
		Print("warning: running prune without a cache, this may be very slow!\n")
	}

	Verbosef("loading all snapshots...\n")
	snapshots, err := restic.LoadAllSnapshots(gopts.ctx, repo, ignoreSnapshots)
	if err != nil {
		return err
	}

	Verbosef("loading indexes...\n")
	err = repo.LoadIndex(gopts.ctx)
	if err != nil {
		return err
	}

	usedBlobs, err := getUsedBlobs(gopts, repo, snapshots)
	if err != nil {
		return err
	}

	return prune(opts, gopts, repo, usedBlobs)
}

type packInfo struct {
	usedBlobs      uint
	unusedBlobs    uint
	duplicateBlobs uint
	usedSize       uint64
	unusedSize     uint64
	tpe            restic.BlobType
}

type packInfoWithID struct {
	ID restic.ID
	packInfo
}

// prune selects which files to rewrite and then does that. The map usedBlobs is
// modified in the process.
func prune(opts PruneOptions, gopts GlobalOptions, repo restic.Repository, usedBlobs restic.BlobSet) error {
	ctx := gopts.ctx

	var stats struct {
		blobs struct {
			used      uint
			duplicate uint
			unused    uint
			remove    uint
			repack    uint
			repackrm  uint
		}
		size struct {
			used      uint64
			duplicate uint64
			unused    uint64
			remove    uint64
			repack    uint64
			repackrm  uint64
			unref     uint64
		}
		packs struct {
			used       uint
			unused     uint
			partlyUsed uint
			keep       uint
		}
	}

	Verbosef("searching used packs...\n")

	keepBlobs := restic.NewBlobSet()
	duplicateBlobs := restic.NewBlobSet()

	// iterate over all blobs in index to find out which blobs are duplicates
	for blob := range repo.Index().Each(ctx) {
		bh := blob.BlobHandle
		size := uint64(blob.Length)
		switch {
		case usedBlobs.Has(bh): // used blob, move to keepBlobs
			usedBlobs.Delete(bh)
			keepBlobs.Insert(bh)
			stats.size.used += size
			stats.blobs.used++
		case keepBlobs.Has(bh): // duplicate blob
			duplicateBlobs.Insert(bh)
			stats.size.duplicate += size
			stats.blobs.duplicate++
		default:
			stats.size.unused += size
			stats.blobs.unused++
		}
	}

	// Check if all used blobs have been found in index
	if len(usedBlobs) != 0 {
		Warnf("%v not found in the new index\n"+
			"Data blobs seem to be missing, aborting prune to prevent further data loss!\n"+
			"Please report this error (along with the output of the 'prune' run) at\n"+
			"https://github.com/restic/restic/issues/new/choose", usedBlobs)
		return errorIndexIncomplete
	}

	indexPack := make(map[restic.ID]packInfo)

	// save computed pack header size
	for pid, hdrSize := range repo.Index().PackSize(ctx, true) {
		// initialize tpe with NumBlobTypes to indicate it's not set
		indexPack[pid] = packInfo{tpe: restic.NumBlobTypes, usedSize: uint64(hdrSize)}
	}

	// iterate over all blobs in index to generate packInfo
	for blob := range repo.Index().Each(ctx) {
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
		switch {
		case duplicateBlobs.Has(bh): // duplicate blob
			ip.usedSize += size
			ip.duplicateBlobs++
		case keepBlobs.Has(bh): // used blob, not duplicate
			ip.usedSize += size
			ip.usedBlobs++
		default: // unused blob
			ip.unusedSize += size
			ip.unusedBlobs++
		}
		// update indexPack
		indexPack[blob.PackID] = ip
	}

	Verbosef("collecting packs for deletion and repacking\n")
	removePacksFirst := restic.NewIDSet()
	removePacks := restic.NewIDSet()
	repackPacks := restic.NewIDSet()

	var repackCandidates []packInfoWithID

	// loop over all packs and decide what to do
	bar := newProgressMax(!gopts.Quiet, uint64(len(indexPack)), "packs processed")
	err := repo.List(ctx, restic.PackFile, func(id restic.ID, packSize int64) error {
		p, ok := indexPack[id]
		if !ok {
			// Pack was not referenced in index and is not used  => immediately remove!
			Verboseff("will remove pack %v as it is unused and not indexed\n", id.Str())
			removePacksFirst.Insert(id)
			stats.size.unref += uint64(packSize)
			return nil
		}

		if p.unusedSize+p.usedSize != uint64(packSize) {
			Warnf("pack %s: calculated size %d does not match real size %d\nRun 'restic rebuild-index'.",
				id.Str(), p.unusedSize+p.usedSize, packSize)
			return errorSizeNotMatching
		}

		// statistics
		switch {
		case p.usedBlobs == 0 && p.duplicateBlobs == 0:
			stats.packs.unused++
		case p.unusedBlobs == 0:
			stats.packs.used++
		default:
			stats.packs.partlyUsed++
		}

		// decide what to do
		switch {
		case p.usedBlobs == 0 && p.duplicateBlobs == 0:
			// All blobs in pack are no longer used => remove pack!
			removePacks.Insert(id)
			stats.blobs.remove += p.unusedBlobs
			stats.size.remove += p.unusedSize

		case opts.RepackCachableOnly && p.tpe == restic.DataBlob:
			// if this is a data pack and --repack-cacheable-only is set => keep pack!
			stats.packs.keep++

		case p.unusedBlobs == 0 && p.duplicateBlobs == 0 && p.tpe != restic.InvalidBlob:
			// All blobs in pack are used and not duplicates/mixed => keep pack!
			stats.packs.keep++

		default:
			// all other packs are candidates for repacking
			repackCandidates = append(repackCandidates, packInfoWithID{ID: id, packInfo: p})
		}

		delete(indexPack, id)
		bar.Add(1)
		return nil
	})
	bar.Done()
	if err != nil {
		return err
	}

	if len(indexPack) != 0 {
		Warnf("The index references pack files which are missing from the repository: %v\n", indexPack)
		return errorPacksMissing
	}

	repackAllPacksWithDuplicates := true

	// calculate limit for number of unused bytes in the repo after repacking
	maxUnusedSizeAfter := opts.maxUnusedBytes(stats.size.used)

	// Sort repackCandidates such that packs with highest ratio unused/used space are picked first.
	// This is equivalent to sorting by unused / total space.
	// Instead of unused[i] / used[i] > unused[j] / used[j] we use
	// unused[i] * used[j] > unused[j] * used[i] as uint32*uint32 < uint64
	// Morover duplicates and mixed are sorted to the beginning
	sort.Slice(repackCandidates, func(i, j int) bool {
		pi := repackCandidates[i].packInfo
		pj := repackCandidates[j].packInfo
		switch {
		case pi.duplicateBlobs > 0 && pj.duplicateBlobs == 0:
			return true
		case pj.duplicateBlobs > 0 && pi.duplicateBlobs == 0:
			return false
		case pi.tpe == restic.InvalidBlob && pj.tpe != restic.InvalidBlob:
			return true
		case pj.tpe == restic.InvalidBlob && pi.tpe != restic.InvalidBlob:
			return false
		}
		return pi.unusedSize*pj.usedSize > pj.unusedSize*pi.usedSize
	})

	repack := func(id restic.ID, p packInfo) {
		repackPacks.Insert(id)
		stats.blobs.repack += p.unusedBlobs + p.duplicateBlobs + p.usedBlobs
		stats.size.repack += p.unusedSize + p.usedSize
		stats.blobs.repackrm += p.unusedBlobs
		stats.size.repackrm += p.unusedSize
	}

	for _, p := range repackCandidates {
		reachedUnusedSizeAfter := (stats.size.unused-stats.size.remove-stats.size.repackrm < maxUnusedSizeAfter)

		reachedRepackSize := false
		if opts.MaxRepackBytes > 0 {
			reachedRepackSize = stats.size.repack+p.unusedSize+p.usedSize > opts.MaxRepackBytes
		}

		switch {
		case !reachedRepackSize && (p.duplicateBlobs > 0 || p.tpe == restic.InvalidBlob):
			// repacking duplicates/mixed is only limited by repackSize
			repack(p.ID, p.packInfo)

		case reachedUnusedSizeAfter, reachedRepackSize:
			// for all other packs stop repacking if tolerated unused size is reached.
			stats.packs.keep++
			if p.duplicateBlobs > 0 {
				repackAllPacksWithDuplicates = false
			}

		default:
			repack(p.ID, p.packInfo)
		}
	}

	// if all duplicates are repacked, print out correct statistics
	if repackAllPacksWithDuplicates {
		stats.blobs.repackrm += stats.blobs.duplicate
		stats.size.repackrm += stats.size.duplicate
	}

	Verboseff("\nused:        %10d blobs / %s\n", stats.blobs.used, formatBytes(stats.size.used))
	if stats.blobs.duplicate > 0 {
		Verboseff("duplicates:  %10d blobs / %s\n", stats.blobs.duplicate, formatBytes(stats.size.duplicate))
	}
	Verboseff("unused:      %10d blobs / %s\n", stats.blobs.unused, formatBytes(stats.size.unused))
	if stats.size.unref > 0 {
		Verboseff("unreferenced:                   %s\n", formatBytes(stats.size.unref))
	}
	totalBlobs := stats.blobs.used + stats.blobs.unused + stats.blobs.duplicate
	totalSize := stats.size.used + stats.size.duplicate + stats.size.unused + stats.size.unref
	unusedSize := stats.size.duplicate + stats.size.unused
	Verboseff("total:       %10d blobs / %s\n", totalBlobs, formatBytes(totalSize))
	Verboseff("unused size: %s of total size\n", formatPercent(unusedSize, totalSize))

	Verbosef("\nto repack:   %10d blobs / %s\n", stats.blobs.repack, formatBytes(stats.size.repack))
	Verbosef("this removes %10d blobs / %s\n", stats.blobs.repackrm, formatBytes(stats.size.repackrm))
	Verbosef("to delete:   %10d blobs / %s\n", stats.blobs.remove, formatBytes(stats.size.remove+stats.size.unref))
	totalPruneSize := stats.size.remove + stats.size.repackrm + stats.size.unref
	Verbosef("total prune: %10d blobs / %s\n", stats.blobs.remove+stats.blobs.repackrm, formatBytes(totalPruneSize))
	Verbosef("remaining:   %10d blobs / %s\n", totalBlobs-(stats.blobs.remove+stats.blobs.repackrm), formatBytes(totalSize-totalPruneSize))
	unusedAfter := unusedSize - stats.size.remove - stats.size.repackrm
	Verbosef("unused size after prune: %s (%s of remaining size)\n",
		formatBytes(unusedAfter), formatPercent(unusedAfter, totalSize-totalPruneSize))
	Verbosef("\n")
	Verboseff("totally used packs: %10d\n", stats.packs.used)
	Verboseff("partly used packs:  %10d\n", stats.packs.partlyUsed)
	Verboseff("unused packs:       %10d\n\n", stats.packs.unused)

	Verboseff("to keep:   %10d packs\n", stats.packs.keep)
	Verboseff("to repack: %10d packs\n", len(repackPacks))
	Verboseff("to delete: %10d packs\n", len(removePacks))
	if len(removePacksFirst) > 0 {
		Verboseff("to delete: %10d unreferenced packs\n\n", len(removePacksFirst))
	}

	if opts.DryRun {
		if !gopts.JSON && gopts.verbosity >= 2 {
			if len(removePacksFirst) > 0 {
				Printf("Would have removed the following unreferenced packs:\n%v\n\n", removePacksFirst)
			}
			Printf("Would have repacked and removed the following packs:\n%v\n\n", repackPacks)
			Printf("Would have removed the following no longer used packs:\n%v\n\n", removePacks)
		}
		// Always quit here if DryRun was set!
		return nil
	}

	// unreferenced packs can be safely deleted first
	if len(removePacksFirst) != 0 {
		Verbosef("deleting unreferenced packs\n")
		DeleteFiles(gopts, repo, removePacksFirst, restic.PackFile)
	}

	packsAddedByRepack := 0
	if len(repackPacks) != 0 {
		// Remember the number of unique packs before repacking
		packsBeforeRepacking := len(repo.Index().Packs())

		Verbosef("repacking packs\n")
		bar := newProgressMax(!gopts.Quiet, uint64(len(repackPacks)), "packs repacked")
		_, err := repository.Repack(ctx, repo, repackPacks, keepBlobs, bar)
		bar.Done()
		if err != nil {
			return err
		}

		// Since repacking will only add new packs, we can calculate the number
		// of packs like this:
		packsAddedByRepack = len(repo.Index().Packs()) - packsBeforeRepacking

		// Also remove repacked packs
		removePacks.Merge(repackPacks)
	}

	if len(removePacks) != 0 {
		totalpacks := int(stats.packs.used+stats.packs.partlyUsed+stats.packs.unused) -
			len(removePacks) + packsAddedByRepack
		err = rebuildIndexFiles(gopts, repo, removePacks, nil, uint64(totalpacks))
		if err != nil {
			return err
		}

		Verbosef("removing %d old packs\n", len(removePacks))
		DeleteFiles(gopts, repo, removePacks, restic.PackFile)
	}

	Verbosef("done\n")
	return nil
}

func rebuildIndexFiles(gopts GlobalOptions, repo restic.Repository, removePacks restic.IDSet, extraObsolete restic.IDs, packcount uint64) error {
	Verbosef("rebuilding index\n")

	bar := newProgressMax(!gopts.Quiet, packcount, "packs processed")
	obsoleteIndexes, err := (repo.Index()).(*repository.MasterIndex).
		Save(gopts.ctx, repo, removePacks, extraObsolete, bar)
	bar.Done()
	if err != nil {
		return err
	}

	Verbosef("deleting obsolete index files\n")
	return DeleteFilesChecked(gopts, repo, obsoleteIndexes, restic.IndexFile)
}

func getUsedBlobs(gopts GlobalOptions, repo restic.Repository, snapshots []*restic.Snapshot) (usedBlobs restic.BlobSet, err error) {
	ctx := gopts.ctx

	Verbosef("finding data that is still in use for %d snapshots\n", len(snapshots))

	usedBlobs = restic.NewBlobSet()

	bar := newProgressMax(!gopts.Quiet, uint64(len(snapshots)), "snapshots")
	defer bar.Done()
	for _, sn := range snapshots {
		debug.Log("process snapshot %v", sn.ID())

		err = restic.FindUsedBlobs(ctx, repo, *sn.Tree, usedBlobs)
		if err != nil {
			if repo.Backend().IsNotExist(err) {
				return nil, errors.Fatal("unable to load a tree from the repo: " + err.Error())
			}

			return nil, err
		}

		debug.Log("processed snapshot %v", sn.ID())
		bar.Add(1)
	}
	return usedBlobs, nil
}
