package main

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/pack"
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
	DryRun                bool
	UnsafeNoSpaceRecovery string

	unsafeRecovery bool

	MaxUnused      string
	maxUnusedBytes func(used uint64) (unused uint64) // calculates the number of unused bytes after repacking, according to MaxUnused

	MaxRepackSize  string
	MaxRepackBytes uint64

	RepackCachableOnly bool
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
	f.BoolVar(&pruneOptions.RepackUncompressed, "repack-uncompressed", false, "repack all uncompressed data")
}

func verifyPruneOptions(opts *PruneOptions) error {
	opts.MaxRepackBytes = math.MaxUint64
	if len(opts.MaxRepackSize) > 0 {
		size, err := parseSizeStr(opts.MaxRepackSize)
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

func runPrune(opts PruneOptions, gopts GlobalOptions) error {
	err := verifyPruneOptions(&opts)
	if err != nil {
		return err
	}

	if opts.RepackUncompressed && gopts.Compression == repository.CompressionOff {
		return errors.Fatal("disabled compression and `--repack-uncompressed` are mutually exclusive")
	}

	repo, err := OpenRepository(gopts)
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

	Verbosef("loading indexes...\n")
	// loading the index before the snapshots is ok, as we use an exclusive lock here
	err := repo.LoadIndex(gopts.ctx)
	if err != nil {
		return err
	}

	usedBlobs, err := getUsedBlobs(gopts, repo, ignoreSnapshots)
	if err != nil {
		return err
	}

	return prune(opts, gopts, repo, usedBlobs)
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
}

type blobStats struct {
	Used         uint `json:"used"`
	Duplicate    uint `json:"duplicate"`
	Unused       uint `json:"unused"`
	Total        uint `json:"total"`
	Repack       uint `json:"repack"`
	RepackRm     uint `json:"repack_remove"`
	Remove       uint `json:"remove"`
	RemoveTotal  uint `json:"remove_total"`
	Remain       uint `json:"remaining"`
	RemainUnused uint `json:"remaining_unused"`
}

type sizeStats struct {
	Used         uint64 `json:"used"`
	Duplicate    uint64 `json:"duplicate"`
	Unused       uint64 `json:"unused"`
	Unref        uint64 `json:"unreferenced"`
	Total        uint64 `json:"total"`
	Repack       uint64 `json:"repack"`
	RepackRm     uint64 `json:"repack_remove"`
	Remove       uint64 `json:"remove"`
	RemoveTotal  uint64 `json:"remove_total"`
	Remain       uint64 `json:"remaining"`
	RemainUnused uint64 `json:"remaining_unused"`
}

type packStats struct {
	Used        uint `json:"used"`
	Unused      uint `json:"unused"`
	PartlyUsed  uint `json:"partly_used"`
	Unref       uint `json:"unreferenced"`
	Total       uint `json:"total"`
	Keep        uint `json:"keep"`
	Repack      uint `json:"repack"`
	Remove      uint `json:"remove"`
	RemoveTotal uint `json:"remove_total"`
}

type pruneStats struct {
	MessageType string    `json:"message_type"` // "summary"
	Blobs       blobStats `json:"blobs"`
	Size        sizeStats `json:"bytes"`
	Packs       packStats `json:"packfiles"`
}

// prune selects which files to rewrite and then does that. The map usedBlobs is
// modified in the process.
func prune(opts PruneOptions, gopts GlobalOptions, repo restic.Repository, usedBlobs restic.BlobSet) error {
	ctx := gopts.ctx

	var stats pruneStats

	Verbosef("searching used packs...\n")

	keepBlobs := restic.NewBlobSet()
	duplicateBlobs := make(map[restic.BlobHandle]uint8)

	// iterate over all blobs in index to find out which blobs are duplicates
	for blob := range repo.Index().Each(ctx) {
		bh := blob.BlobHandle
		size := uint64(blob.Length)
		switch {
		case usedBlobs.Has(bh): // used blob, move to keepBlobs
			usedBlobs.Delete(bh)
			keepBlobs.Insert(bh)
			stats.Size.Used += size
			stats.Blobs.Used++
		case keepBlobs.Has(bh): // duplicate blob
			count, ok := duplicateBlobs[bh]
			if !ok {
				count = 2 // this one is already the second blob!
			} else if count < math.MaxUint8 {
				// don't overflow, but saturate count at 255
				// this can lead to a non-optimal pack selection, but won't cause
				// problems otherwise
				count++
			}
			duplicateBlobs[bh] = count
			stats.Size.Duplicate += size
			stats.Blobs.Duplicate++
		default:
			stats.Size.Unused += size
			stats.Blobs.Unused++
		}
	}

	// Check if all used blobs have been found in index
	if len(usedBlobs) != 0 {
		Warnf("%v not found in the index\n\n"+
			"Integrity check failed: Data seems to be missing.\n"+
			"Will not start prune to prevent (additional) data loss!\n"+
			"Please report this error (along with the output of the 'prune' run) at\n"+
			"https://github.com/restic/restic/issues/new/choose\n", usedBlobs)
		return errorIndexIncomplete
	}

	indexPack := make(map[restic.ID]packInfo)

	// save computed pack header size
	for pid, hdrSize := range pack.Size(ctx, repo.Index(), true) {
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
		_, isDuplicate := duplicateBlobs[bh]
		switch {
		case isDuplicate: // duplicate blobs will be handled later
		case keepBlobs.Has(bh): // used blob, not duplicate
			ip.usedSize += size
			ip.usedBlobs++
		default: // unused blob
			ip.unusedSize += size
			ip.unusedBlobs++
		}
		if !blob.IsCompressed() {
			ip.uncompressed = true
		}
		// update indexPack
		indexPack[blob.PackID] = ip
	}

	// if duplicate blobs exist, those will be set to either "used" or "unused":
	// - mark only one occurence of duplicate blobs as used
	// - if there are already some used blobs in a pack, possibly mark duplicates in this pack as "used"
	// - if there are no used blobs in a pack, possibly mark duplicates as "unused"
	if len(duplicateBlobs) > 0 {
		// iterate again over all blobs in index (this is pretty cheap, all in-mem)
		for blob := range repo.Index().Each(ctx) {
			bh := blob.BlobHandle
			count, isDuplicate := duplicateBlobs[bh]
			if !isDuplicate {
				continue
			}

			ip := indexPack[blob.PackID]
			size := uint64(blob.Length)
			switch {
			case count == 0:
				// used duplicate exists ->  mark as unused
				ip.unusedSize += size
				ip.unusedBlobs++
			case ip.usedBlobs > 0, count == 1:
				// other used blobs in pack or "last" occurency ->  mark as used
				ip.usedSize += size
				ip.usedBlobs++
				// let other occurences be marked as unused
				duplicateBlobs[bh] = 0
			default:
				// mark as unused and decrease counter
				ip.unusedSize += size
				ip.unusedBlobs++
				duplicateBlobs[bh] = count - 1
			}
			// update indexPack
			indexPack[blob.PackID] = ip
		}
	}

	Verbosef("collecting packs for deletion and repacking\n")
	removePacksFirst := restic.NewIDSet()
	removePacks := restic.NewIDSet()
	repackPacks := restic.NewIDSet()

	var repackCandidates []packInfoWithID
	repoVersion := repo.Config().Version

	// loop over all packs and decide what to do
	bar := newProgressMax(!gopts.Quiet, uint64(len(indexPack)), "packs processed")
	err := repo.List(ctx, restic.PackFile, func(id restic.ID, packSize int64) error {
		p, ok := indexPack[id]
		if !ok {
			// Pack was not referenced in index and is not used  => immediately remove!
			Verboseff("will remove pack %v as it is unused and not indexed\n", id.Str())
			removePacksFirst.Insert(id)
			stats.Size.Unref += uint64(packSize)
			return nil
		}

		if p.unusedSize+p.usedSize != uint64(packSize) && p.usedBlobs != 0 {
			// Pack size does not fit and pack is needed => error
			// If the pack is not needed, this is no error, the pack can
			// and will be simply removed, see below.
			Warnf("pack %s: calculated size %d does not match real size %d\nRun 'restic rebuild-index'.\n",
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

		mustCompress := false
		if repoVersion >= 2 {
			// repo v2: always repack tree blobs if uncompressed
			// compress data blobs if requested
			mustCompress = (p.tpe == restic.TreeBlob || opts.RepackUncompressed) && p.uncompressed
		}
		// use a flag that pack must be compressed
		p.uncompressed = mustCompress

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
			// All blobs in pack are used and not mixed => keep pack!
			stats.Packs.Keep++

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
		Warnf("The index references %d needed pack files which are missing from the repository:\n", len(indexPack))
		for id := range indexPack {
			Warnf("  %v\n", id)
		}
		return errorPacksMissing
	}
	if len(ignorePacks) != 0 {
		Warnf("Missing but unneeded pack files are referenced in the index, will be repaired\n")
		for id := range ignorePacks {
			Warnf("will forget missing pack file %v\n", id)
		}
	}

	// calculate limit for number of unused bytes in the repo after repacking
	maxUnusedSizeAfter := opts.maxUnusedBytes(stats.Size.Used)

	// Sort repackCandidates such that packs with highest ratio unused/used space are picked first.
	// This is equivalent to sorting by unused / total space.
	// Instead of unused[i] / used[i] > unused[j] / used[j] we use
	// unused[i] * used[j] > unused[j] * used[i] as uint32*uint32 < uint64
	// Morover packs containing trees are sorted to the beginning
	sort.Slice(repackCandidates, func(i, j int) bool {
		pi := repackCandidates[i].packInfo
		pj := repackCandidates[j].packInfo
		switch {
		case pi.tpe != restic.DataBlob && pj.tpe == restic.DataBlob:
			return true
		case pj.tpe != restic.DataBlob && pi.tpe == restic.DataBlob:
			return false
		}
		return pi.unusedSize*pj.usedSize > pj.unusedSize*pi.usedSize
	})

	repack := func(id restic.ID, p packInfo) {
		repackPacks.Insert(id)
		stats.Blobs.Repack += p.unusedBlobs + p.usedBlobs
		stats.Size.Repack += p.unusedSize + p.usedSize
		stats.Blobs.RepackRm += p.unusedBlobs
		stats.Size.RepackRm += p.unusedSize
	}

	for _, p := range repackCandidates {
		reachedUnusedSizeAfter := (stats.Size.Unused-stats.Size.Remove-stats.Size.RepackRm < maxUnusedSizeAfter)
		reachedRepackSize := stats.Size.Repack+p.unusedSize+p.usedSize >= opts.MaxRepackBytes

		switch {
		case reachedRepackSize:
			stats.Packs.Keep++

		case p.tpe != restic.DataBlob, p.uncompressed:
			// repacking non-data packs / uncompressed-trees is only limited by repackSize
			repack(p.ID, p.packInfo)

		case reachedUnusedSizeAfter:
			// for all other packs stop repacking if tolerated unused size is reached.
			stats.Packs.Keep++

		default:
			repack(p.ID, p.packInfo)
		}
	}

	if len(repackPacks) != 0 {
		// when repacking, we do not want to keep blobs which are
		// already contained in kept packs, so delete them from keepBlobs
		for blob := range repo.Index().Each(ctx) {
			if removePacks.Has(blob.PackID) || repackPacks.Has(blob.PackID) {
				continue
			}
			keepBlobs.Delete(blob.BlobHandle)
		}
	} else {
		// keepBlobs is only needed if packs are repacked
		keepBlobs = nil
	}

	// calculate totals for statistics
	stats.MessageType = "summary"
	stats.Blobs.Total = stats.Blobs.Used + stats.Blobs.Unused + stats.Blobs.Duplicate
	stats.Blobs.RemoveTotal = stats.Blobs.Remove + stats.Blobs.RepackRm
	stats.Blobs.Remain = stats.Blobs.Total - stats.Blobs.RemoveTotal
	stats.Size.Total = stats.Size.Used + stats.Size.Duplicate + stats.Size.Unused + stats.Size.Unref
	stats.Size.Unused = stats.Size.Duplicate + stats.Size.Unused
	stats.Size.RemoveTotal = stats.Size.Remove + stats.Size.RepackRm + stats.Size.Unref
	stats.Size.Remain = stats.Size.Total - stats.Size.RemoveTotal
	stats.Size.RemainUnused = stats.Size.Unused - stats.Size.Remove - stats.Size.RepackRm
	stats.Packs.Unref = uint(len(removePacksFirst))
	stats.Packs.Total = stats.Packs.Used + stats.Packs.PartlyUsed + stats.Packs.Unused + stats.Packs.Unref
	stats.Packs.Repack = uint(len(repackPacks))
	stats.Packs.Remove = uint(len(removePacks))
	stats.Packs.RemoveTotal = stats.Packs.Unref + stats.Packs.Remove

	Verboseff("\nused:         %10d blobs / %s\n", stats.Blobs.Used, formatBytes(stats.Size.Used))
	if stats.Blobs.Duplicate > 0 {
		Verboseff("duplicates:   %10d blobs / %s\n", stats.Blobs.Duplicate, formatBytes(stats.Size.Duplicate))
	}
	Verboseff("unused:       %10d blobs / %s\n", stats.Blobs.Unused, formatBytes(stats.Size.Unused))
	if stats.Size.Unref > 0 {
		Verboseff("unreferenced:                    %s\n", formatBytes(stats.Size.Unref))
	}
	Verboseff("total:        %10d blobs / %s\n", stats.Blobs.Total, formatBytes(stats.Size.Total))
	Verboseff("unused size: %s of total size\n", formatPercent(stats.Size.Unused, stats.Size.Total))

	Verbosef("\nto repack:    %10d blobs / %s\n", stats.Blobs.Repack, formatBytes(stats.Size.Repack))
	Verbosef("this removes: %10d blobs / %s\n", stats.Blobs.RepackRm, formatBytes(stats.Size.RepackRm))
	Verbosef("to delete:    %10d blobs / %s\n", stats.Blobs.Remove, formatBytes(stats.Size.Remove+stats.Size.Unref))
	Verbosef("total prune:  %10d blobs / %s\n", stats.Blobs.RemoveTotal, formatBytes(stats.Size.RemoveTotal))
	Verbosef("remaining:    %10d blobs / %s\n", stats.Blobs.Remain, formatBytes(stats.Size.Remain))
	Verbosef("unused size after prune: %s (%s of remaining size)\n",
		formatBytes(stats.Size.RemainUnused), formatPercent(stats.Size.RemainUnused, stats.Size.Remain))
	Verbosef("\n")
	Verboseff("totally used packs: %10d\n", stats.Packs.Used)
	Verboseff("partly used packs:  %10d\n", stats.Packs.PartlyUsed)
	Verboseff("unused packs:       %10d\n\n", stats.Packs.Unused)

	Verboseff("to keep:      %10d packs\n", stats.Packs.Keep)
	Verboseff("to repack:    %10d packs\n", stats.Packs.Repack)
	Verboseff("to delete:    %10d packs\n", stats.Packs.Remove)
	if stats.Packs.Unref > 0 {
		Verboseff("to delete:    %10d unreferenced packs\n\n", stats.Packs.Unref)
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

	if len(repackPacks) != 0 {
		Verbosef("repacking packs\n")
		bar := newProgressMax(!gopts.Quiet, uint64(len(repackPacks)), "packs repacked")
		_, err := repository.Repack(ctx, repo, repo, repackPacks, keepBlobs, bar)
		bar.Done()
		if err != nil {
			return errors.Fatalf("%s", err)
		}

		// Also remove repacked packs
		removePacks.Merge(repackPacks)
	}

	if len(ignorePacks) == 0 {
		ignorePacks = removePacks
	} else {
		ignorePacks.Merge(removePacks)
	}

	if opts.unsafeRecovery {
		Verbosef("deleting index files\n")
		indexFiles := repo.Index().(*repository.MasterIndex).IDs()
		err = DeleteFilesChecked(gopts, repo, indexFiles, restic.IndexFile)
		if err != nil {
			return errors.Fatalf("%s", err)
		}
	} else if len(ignorePacks) != 0 {
		err = rebuildIndexFiles(gopts, repo, ignorePacks, nil)
		if err != nil {
			return errors.Fatalf("%s", err)
		}
	}

	if len(removePacks) != 0 {
		Verbosef("removing %d old packs\n", len(removePacks))
		DeleteFiles(gopts, repo, removePacks, restic.PackFile)
	}

	if opts.unsafeRecovery {
		_, err = writeIndexFiles(gopts, repo, ignorePacks, nil)
		if err != nil {
			return errors.Fatalf("%s", err)
		}
	}

	Verbosef("done\n")
	return nil
}

func writeIndexFiles(gopts GlobalOptions, repo restic.Repository, removePacks restic.IDSet, extraObsolete restic.IDs) (restic.IDSet, error) {
	Verbosef("rebuilding index\n")

	bar := newProgressMax(!gopts.Quiet, 0, "packs processed")
	obsoleteIndexes, err := repo.Index().Save(gopts.ctx, repo, removePacks, extraObsolete, bar)
	bar.Done()
	return obsoleteIndexes, err
}

func rebuildIndexFiles(gopts GlobalOptions, repo restic.Repository, removePacks restic.IDSet, extraObsolete restic.IDs) error {
	obsoleteIndexes, err := writeIndexFiles(gopts, repo, removePacks, extraObsolete)
	if err != nil {
		return err
	}

	Verbosef("deleting obsolete index files\n")
	return DeleteFilesChecked(gopts, repo, obsoleteIndexes, restic.IndexFile)
}

func getUsedBlobs(gopts GlobalOptions, repo restic.Repository, ignoreSnapshots restic.IDSet) (usedBlobs restic.BlobSet, err error) {
	ctx := gopts.ctx

	var snapshotTrees restic.IDs
	Verbosef("loading all snapshots...\n")
	err = restic.ForAllSnapshots(gopts.ctx, repo.Backend(), repo, ignoreSnapshots,
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

	usedBlobs = restic.NewBlobSet()

	bar := newProgressMax(!gopts.Quiet, uint64(len(snapshotTrees)), "snapshots")
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
