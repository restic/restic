package repository

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository/index"
	"github.com/restic/restic/internal/repository/pack"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/progress"
)

var ErrIndexIncomplete = errors.Fatal("index is not complete")
var ErrPacksMissing = errors.Fatal("packs from index missing in repo")
var ErrSizeNotMatching = errors.Fatal("pack size does not match calculated size from index")

// PruneOptions collects all options for the cleanup command.
type PruneOptions struct {
	DryRun         bool
	UnsafeRecovery bool

	MaxUnusedBytes func(used uint64) (unused uint64) // calculates the number of unused bytes after repacking, according to MaxUnused
	MaxRepackBytes uint64

	RepackCacheableOnly bool
	RepackSmall         bool
	RepackUncompressed  bool
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
	removePacksFirst restic.IDSet                // packs to remove first (unreferenced packs)
	repackPacks      restic.IDSet                // packs to repack
	keepBlobs        *index.AssociatedSet[uint8] // blobs to keep during repacking
	removePacks      restic.IDSet                // packs to remove
	ignorePacks      restic.IDSet                // packs to ignore when rebuilding the index

	repo  *Repository
	stats PruneStats
	opts  PruneOptions
}

type packInfo struct {
	usedBlobs      uint
	unusedBlobs    uint
	duplicateBlobs uint
	usedSize       uint64
	unusedSize     uint64

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
func PlanPrune(ctx context.Context, opts PruneOptions, repo *Repository, getUsedBlobs func(ctx context.Context, repo restic.Repository, usedBlobs restic.FindBlobSet) error, printer progress.Printer) (*PrunePlan, error) {
	var stats PruneStats

	if opts.UnsafeRecovery {
		// prevent repacking data to make sure users cannot get stuck.
		opts.MaxRepackBytes = 0
	}
	if repo.Connections() < 2 {
		return nil, fmt.Errorf("prune requires a backend connection limit of at least two")
	}
	if repo.Config().Version < 2 && opts.RepackUncompressed {
		return nil, fmt.Errorf("compression requires at least repository format version 2")
	}

	usedBlobs := index.NewAssociatedSet[uint8](repo.idx)
	err := getUsedBlobs(ctx, repo, usedBlobs)
	if err != nil {
		return nil, err
	}

	printer.P("searching used packs...\n")
	keepBlobs, indexPack, err := packInfoFromIndex(ctx, repo, usedBlobs, &stats, printer)
	if err != nil {
		return nil, err
	}

	printer.P("collecting packs for deletion and repacking\n")
	plan, err := decidePackAction(ctx, opts, repo, indexPack, &stats, printer)
	if err != nil {
		return nil, err
	}

	if len(plan.repackPacks) != 0 {
		// when repacking, we do not want to keep blobs which are
		// already contained in kept packs, so delete them from keepBlobs
		err := repo.ListBlobs(ctx, func(blob restic.PackedBlob) {
			if plan.removePacks.Has(blob.PackID) || plan.repackPacks.Has(blob.PackID) {
				return
			}
			keepBlobs.Delete(blob.BlobHandle)
		})
		if err != nil {
			return nil, err
		}
	} else {
		// keepBlobs is only needed if packs are repacked
		keepBlobs = nil
	}
	plan.keepBlobs = keepBlobs

	plan.repo = repo
	plan.stats = stats
	plan.opts = opts

	return &plan, nil
}

func packInfoFromIndex(ctx context.Context, idx restic.ListBlobser, usedBlobs *index.AssociatedSet[uint8], stats *PruneStats, printer progress.Printer) (*index.AssociatedSet[uint8], map[restic.ID]packInfo, error) {
	// iterate over all blobs in index to find out which blobs are duplicates
	// The counter in usedBlobs describes how many instances of the blob exist in the repository index
	// Thus 0 == blob is missing, 1 == blob exists once, >= 2 == duplicates exist
	err := idx.ListBlobs(ctx, func(blob restic.PackedBlob) {
		bh := blob.BlobHandle
		count, ok := usedBlobs.Get(bh)
		if ok {
			if count < math.MaxUint8 {
				// don't overflow, but saturate count at 255
				// this can lead to a non-optimal pack selection, but won't cause
				// problems otherwise
				count++
			}

			usedBlobs.Set(bh, count)
		}
	})
	if err != nil {
		return nil, nil, err
	}

	// Check if all used blobs have been found in index
	missingBlobs := restic.NewBlobSet()
	usedBlobs.For(func(bh restic.BlobHandle, count uint8) {
		if count == 0 {
			// blob does not exist in any pack files
			missingBlobs.Insert(bh)
		}
	})

	if len(missingBlobs) != 0 {
		printer.E("%v not found in the index\n\n"+
			"Integrity check failed: Data seems to be missing.\n"+
			"Will not start prune to prevent (additional) data loss!\n"+
			"Please report this error (along with the output of the 'prune' run) at\n"+
			"https://github.com/restic/restic/issues/new/choose\n", missingBlobs)
		return nil, nil, ErrIndexIncomplete
	}

	indexPack := make(map[restic.ID]packInfo)

	// save computed pack header size
	sz, err := pack.Size(ctx, idx, true)
	if err != nil {
		return nil, nil, err
	}
	for pid, hdrSize := range sz {
		// initialize tpe with NumBlobTypes to indicate it's not set
		indexPack[pid] = packInfo{tpe: restic.NumBlobTypes, usedSize: uint64(hdrSize)}
	}

	hasDuplicates := false
	// iterate over all blobs in index to generate packInfo
	err = idx.ListBlobs(ctx, func(blob restic.PackedBlob) {
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
		dupCount, _ := usedBlobs.Get(bh)
		switch {
		case dupCount >= 2:
			hasDuplicates = true
			// mark as unused for now, we will later on select one copy
			ip.unusedSize += size
			ip.unusedBlobs++
			ip.duplicateBlobs++

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
	if err != nil {
		return nil, nil, err
	}

	// if duplicate blobs exist, those will be set to either "used" or "unused":
	// - mark only one occurrence of duplicate blobs as used
	// - if there are already some used blobs in a pack, possibly mark duplicates in this pack as "used"
	// - if a pack only consists of duplicates (which by definition are used blobs), mark it as "used". This
	//   ensures that already rewritten packs are kept.
	// - if there are no used blobs in a pack, possibly mark duplicates as "unused"
	if hasDuplicates {
		// iterate again over all blobs in index (this is pretty cheap, all in-mem)
		err = idx.ListBlobs(ctx, func(blob restic.PackedBlob) {
			bh := blob.BlobHandle
			count, ok := usedBlobs.Get(bh)
			// skip non-duplicate, aka. normal blobs
			// count == 0 is used to mark that this was a duplicate blob with only a single occurrence remaining
			if !ok || count == 1 {
				return
			}

			ip := indexPack[blob.PackID]
			size := uint64(blob.Length)
			switch {
			case ip.usedBlobs > 0, (ip.duplicateBlobs == ip.unusedBlobs), count == 0:
				// other used blobs in pack, only duplicate blobs or "last" occurrence ->  transition to used
				// a pack file created by an interrupted prune run will consist of only duplicate blobs
				// thus select such already repacked pack files
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
				usedBlobs.Set(bh, 1)
			default:
				// remain unused and decrease counter
				count--
				if count == 1 {
					// setting count to 1 would lead to forgetting that this blob had duplicates
					// thus use the special value zero. This will select the last instance of the blob for keeping.
					count = 0
				}
				usedBlobs.Set(bh, count)
			}
			// update indexPack
			indexPack[blob.PackID] = ip
		})
		if err != nil {
			return nil, nil, err
		}
	}

	// Sanity check. If no duplicates exist, all blobs have value 1. After handling
	// duplicates, this also applies to duplicates.
	usedBlobs.For(func(_ restic.BlobHandle, count uint8) {
		if count != 1 {
			panic("internal error during blob selection")
		}
	})

	return usedBlobs, indexPack, nil
}

func decidePackAction(ctx context.Context, opts PruneOptions, repo *Repository, indexPack map[restic.ID]packInfo, stats *PruneStats, printer progress.Printer) (PrunePlan, error) {
	removePacksFirst := restic.NewIDSet()
	removePacks := restic.NewIDSet()
	repackPacks := restic.NewIDSet()

	var repackCandidates []packInfoWithID
	var repackSmallCandidates []packInfoWithID
	repoVersion := repo.Config().Version
	// only repack very small files by default
	targetPackSize := repo.packSize() / 25
	if opts.RepackSmall {
		// consider files with at least 80% of the target size as large enough
		targetPackSize = repo.packSize() / 5 * 4
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
			return ErrSizeNotMatching
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

		case opts.RepackCacheableOnly && p.tpe == restic.DataBlob:
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
		return PrunePlan{}, ErrPacksMissing
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
	// Moreover packs containing trees and too short packs are sorted to the beginning
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
	maxUnusedSizeAfter := opts.MaxUnusedBytes(stats.Size.Used)

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

func (plan *PrunePlan) Stats() PruneStats {
	return plan.stats
}

// Execute does the actual pruning:
// - remove unreferenced packs first
// - repack given pack files while keeping the given blobs
// - rebuild the index while ignoring all files that will be deleted
// - delete the files
// plan.removePacks and plan.ignorePacks are modified in this function.
func (plan *PrunePlan) Execute(ctx context.Context, printer progress.Printer) error {
	if plan.opts.DryRun {
		printer.V("Repeated prune dry-runs can report slightly different amounts of data to keep or repack. This is expected behavior.\n\n")
		if len(plan.removePacksFirst) > 0 {
			printer.V("Would have removed the following unreferenced packs:\n%v\n\n", plan.removePacksFirst)
		}
		printer.V("Would have repacked and removed the following packs:\n%v\n\n", plan.repackPacks)
		printer.V("Would have removed the following no longer used packs:\n%v\n\n", plan.removePacks)
		// Always quit here if DryRun was set!
		return nil
	}

	repo := plan.repo
	// make sure the plan can only be used once
	plan.repo = nil

	// unreferenced packs can be safely deleted first
	if len(plan.removePacksFirst) != 0 {
		printer.P("deleting unreferenced packs\n")
		_ = deleteFiles(ctx, true, repo, plan.removePacksFirst, restic.PackFile, printer)
		// forget unused data
		plan.removePacksFirst = nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if len(plan.repackPacks) != 0 {
		printer.P("repacking packs\n")
		bar := printer.NewCounter("packs repacked")
		bar.SetMax(uint64(len(plan.repackPacks)))
		_, err := Repack(ctx, repo, repo, plan.repackPacks, plan.keepBlobs, bar)
		bar.Done()
		if err != nil {
			return errors.Fatal(err.Error())
		}

		// Also remove repacked packs
		plan.removePacks.Merge(plan.repackPacks)
		// forget unused data
		plan.repackPacks = nil

		if plan.keepBlobs.Len() != 0 {
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

	if plan.opts.UnsafeRecovery {
		printer.P("deleting index files\n")
		indexFiles := repo.idx.IDs()
		err := deleteFiles(ctx, false, repo, indexFiles, restic.IndexFile, printer)
		if err != nil {
			return errors.Fatalf("%s", err)
		}
	} else if len(plan.ignorePacks) != 0 {
		err := rewriteIndexFiles(ctx, repo, plan.ignorePacks, nil, nil, printer)
		if err != nil {
			return errors.Fatalf("%s", err)
		}
	}

	if len(plan.removePacks) != 0 {
		printer.P("removing %d old packs\n", len(plan.removePacks))
		_ = deleteFiles(ctx, true, repo, plan.removePacks, restic.PackFile, printer)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if plan.opts.UnsafeRecovery {
		err := repo.idx.SaveFallback(ctx, repo, plan.ignorePacks, printer.NewCounter("packs processed"))
		if err != nil {
			return errors.Fatalf("%s", err)
		}
	}

	// drop outdated in-memory index
	repo.clearIndex()

	printer.P("done\n")
	return nil
}

// deleteFiles deletes the given fileList of fileType in parallel
// if ignoreError=true, it will print a warning if there was an error, else it will abort.
func deleteFiles(ctx context.Context, ignoreError bool, repo restic.RemoverUnpacked, fileList restic.IDSet, fileType restic.FileType, printer progress.Printer) error {
	bar := printer.NewCounter("files deleted")
	defer bar.Done()

	return restic.ParallelRemove(ctx, repo, fileList, fileType, func(id restic.ID, err error) error {
		if err != nil {
			printer.E("unable to remove %v/%v from the repository\n", fileType, id)
			if !ignoreError {
				return err
			}
		}
		printer.VV("removed %v/%v\n", fileType, id)
		return nil
	}, bar)
}
