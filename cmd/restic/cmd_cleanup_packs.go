package main

import (
	"context"
	"sync"

	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/pack"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"

	"github.com/spf13/cobra"
)

var cmdCleanupPacks = &cobra.Command{
	Use:   "cleanup-packs [flags]",
	Short: "Remove packs not in index",
	Long: `
The "cleanup-packs" cleans up data in packs
that is not referenced in any index files.

When calling this command without flags, only packs
that are completely unused are deleted.
You can specify additional conditions to repack
packs that are only partly used or too small.
These packs will be downloaded and uploaded again which can be
quite time-consuming for remote repositories.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCleanupPacks(cleanupPacksOptions, globalOptions)
	},
}

// CleanupIndexOptions collects all options for the cleanup-index command.
type CleanupPacksOptions struct {
	DryRun            bool
	DataUnusedPercent int8
	DataUnusedSpace   int64
	DataUsedSpace     int64
	TreeUnusedPercent int8
	TreeUnusedSpace   int64
	TreeUsedSpace     int64
	RepackMixed       bool
}

var cleanupPacksOptions CleanupPacksOptions

func init() {
	cmdRoot.AddCommand(cmdCleanupPacks)

	f := cmdCleanupPacks.Flags()
	f.BoolVarP(&cleanupPacksOptions.DryRun, "dry-run", "n", false, "do not delete anything, just print what would be done")
	f.Int8Var(&cleanupPacksOptions.DataUnusedPercent, "data-unused-percent", -1, "if > 0, repack data packs with >= given % unused space")
	f.Int64Var(&cleanupPacksOptions.DataUnusedSpace, "data-unused-space", -1, "if > 0, repack data packs with >= given bytes unused space")
	f.Int64Var(&cleanupPacksOptions.DataUsedSpace, "data-used-space", -1, "repack data packs with <= given bytes used space")
	f.Int8Var(&cleanupPacksOptions.TreeUnusedPercent, "tree-unused-percent", -1, "if > 0, repack tree packs with >= given % unused space")
	f.Int64Var(&cleanupPacksOptions.TreeUnusedSpace, "tree-unused-space", 1<<20, "if > 0, repack tree packs with >= given bytes unused space")
	f.Int64Var(&cleanupPacksOptions.TreeUsedSpace, "tree-used-space", 1<<16, "repack tree packs with <= given bytes used space")
	f.BoolVar(&cleanupPacksOptions.RepackMixed, "repack-mixed", true, "repack packs that have mixed blob types")
}

func runCleanupPacks(opts CleanupPacksOptions, gopts GlobalOptions) error {
	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	lock, err := lockRepoExclusive(repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	Verbosef("load indexes\n")
	err = repo.LoadIndex(gopts.ctx)
	if err != nil {
		return err
	}

	return CleanupPacks(opts, gopts, repo)
}

type packInfo struct {
	length uint
	tpe    restic.BlobType
}

func CleanupPacks(opts CleanupPacksOptions, gopts GlobalOptions, repo restic.Repository) error {

	ctx := gopts.ctx

	Verbosef("find packs in index and calculate used size...\n")
	indexPack := make(map[restic.ID]packInfo)
	for blob := range repo.Index().Each(ctx) {
		ip, ok := indexPack[blob.PackID]
		if !ok {
			// Start with 4 bytes overhead per pack (len of header)
			ip = packInfo{length: 4, tpe: blob.Type}
			indexPack[blob.PackID] = ip
		}
		// overhead per blob is the entry Size of the header
		// + crypto overhead
		if ip.tpe != blob.Type {
			ip.tpe = restic.InvalidBlob
		}
		ip.length += blob.Length + pack.EntrySize + crypto.Extension

		indexPack[blob.PackID] = ip
	}

	Verbosef("repack and collect packs for deletion\n")
	removePacks := restic.NewIDSet()
	repackPacks := restic.NewIDSet()
	repackSmallPacks := restic.NewIDSet()
	removeBytes := uint64(0)
	repackBytes := uint64(0)
	repackFreeBytes := uint64(0)

	err := repo.List(ctx, restic.DataFile, func(id restic.ID, packSize int64) error {
		p, ok := indexPack[id]
		usedSize := int64(p.length)
		unusedSize := packSize - usedSize
		unusedPercent := int8((unusedSize * 100) / packSize)

		switch {
		case !ok:
			// Pack not in index! => remove!
			removePacks.Insert(id)
			removeBytes += uint64(packSize)
		case opts.RepackMixed && p.tpe == restic.InvalidBlob,
			p.tpe == restic.DataBlob && opts.DataUnusedPercent > 0 && unusedPercent >= opts.DataUnusedPercent,
			p.tpe == restic.DataBlob && opts.DataUnusedSpace > 0 && unusedSize >= opts.DataUnusedSpace,
			p.tpe == restic.TreeBlob && opts.TreeUnusedPercent > 0 && unusedPercent >= opts.TreeUnusedPercent,
			p.tpe == restic.TreeBlob && opts.TreeUnusedSpace > 0 && unusedSize >= opts.TreeUnusedSpace:
			// repack if pack has mixed blobtypes or fits conditions
			repackPacks.Insert(id)
			repackBytes += uint64(packSize)
			repackFreeBytes += uint64(unusedSize)
		case p.tpe == restic.DataBlob && usedSize <= opts.DataUsedSpace,
			p.tpe == restic.TreeBlob && usedSize <= opts.TreeUsedSpace:
			// repack if pack is too small
			repackSmallPacks.Insert(id)
			repackBytes += uint64(packSize)
			repackFreeBytes += uint64(unusedSize)
		}
		return nil
	})
	if err != nil {
		return err
	}

	if len(repackSmallPacks) == 1 && len(repackPacks) == 0 {
		repackSmallPacks = restic.NewIDSet()
		repackBytes = uint64(0)
		repackFreeBytes = uint64(0)
	}

	Verbosef("found %d unused packs\n", len(removePacks))
	Verbosef("deleting unused packs will free about %s\n\n", formatBytes(removeBytes))

	Verbosef("found %d partly used packs + %d small packs = total %d packs for repacking with %s\n",
		len(repackPacks), len(repackSmallPacks), len(repackPacks), formatBytes(repackBytes))
	Verbosef("repacking packs will free about %s\n", formatBytes(repackFreeBytes))

	Verbosef("cleanup-packs will totally free about %s\n", formatBytes(removeBytes+repackFreeBytes))

	repackPacks.Merge(repackSmallPacks)

	if len(repackPacks) > 0 {
		Verbosef("repacking packs...\n")

		// TODO: With a better index implementation, this information should
		// be saved within the index
		blobsPerPack := make(map[restic.ID][]restic.Blob)
		for blob := range repo.Index().Each(ctx) {
			if repackPacks.Has(blob.PackID) {
				blobsPerPack[blob.PackID] = append(blobsPerPack[blob.PackID],
					restic.Blob{
						ID:     blob.ID,
						Type:   blob.Type,
						Offset: blob.Offset,
						Length: blob.Length,
					})
			}
		}

		// TODO: Parallelize repacking
		for id := range repackPacks {
			if !opts.DryRun {
				blobs := blobsPerPack[id]
				debug.Log("processing pack %v, blobs: %v", id, len(blobs))
				err := repack(ctx, repo, blobs, nil)
				if err != nil {
					return err
				}
			} else {
				if !gopts.JSON && gopts.verbosity >= 2 {
					Verbosef("would have repacked pack %v.\n", id.Str())
				}
			}

			// Also remove repacked pack at the end!
			removePacks.Insert(id)
		}

		err = repo.Flush(ctx)
		if err != nil {
			return err
		}
	}

	if len(repackPacks) > 0 && !opts.DryRun {
		Verbosef("updating index files...\n")

		// TODO: This is a hack to get the index entries of repacked blobs
		// Should be replaced if there is another index implementation
		notFinalIdx := (repo.Index()).(*repository.MasterIndex).NotFinalIndexes()
		if len(notFinalIdx) != 1 {
			return errors.Fatal("should only have one unfinalized index!!")
		}
		repackIndex := notFinalIdx[0]

		err = ChangePacksInIndex(opts, gopts, repo, repackIndex)
		if err != nil {
			return err
		}
	}

	if len(removePacks) != 0 {
		Verbosef("deleting packs (unused and repacked)\n")
		fileHandles := make(chan restic.Handle)
		go func() {
			for packID := range removePacks {
				fileHandles <- restic.Handle{Type: restic.DataFile, Name: packID.String()}
			}
			close(fileHandles)
		}()
		DeleteFiles(gopts, opts.DryRun, repo, len(removePacks), fileHandles)

	}

	Verbosef("done\n")
	return nil
}

const numDeleteWorkers = 8

func DeleteFiles(gopts GlobalOptions, dryrun bool, repo restic.Repository, totalCount int, fileHandles <-chan restic.Handle) {
	var wg sync.WaitGroup
	bar := newProgressMax(!gopts.Quiet, uint64(totalCount), "files deleted")
	bar.Start()
	wg.Add(numDeleteWorkers)
	for i := 0; i < numDeleteWorkers; i++ {
		go func() {
			for h := range fileHandles {
				if !dryrun {
					err := repo.Backend().Remove(gopts.ctx, h)
					if err != nil {
						Warnf("unable to remove file %v from the repository\n", h.Name)
					}
					if !gopts.JSON && gopts.verbosity >= 2 {
						Verbosef("%v was removed.\n", h.Name)
					}
				} else {
					if !gopts.JSON && gopts.verbosity >= 2 {
						Verbosef("would have removed %v.\n", h.Name)
					}
				}
				bar.Report(restic.Stat{Blobs: 1})
			}
			wg.Done()
		}()
	}
	wg.Wait()
	bar.Done()
}

func repack(ctx context.Context, repo restic.Repository, blobs []restic.Blob, p *restic.Progress) (err error) {

	var buf []byte
	for _, entry := range blobs {
		// if blob is not in index, don't write it
		if !repo.Index().Has(entry.ID, entry.Type) {
			debug.Log("not writing blob %v (not in index)", entry.ID)
			continue
		}

		buf = buf[:]
		if uint(len(buf)) < entry.Length {
			buf = make([]byte, entry.Length)
		}
		buf = buf[:entry.Length]

		// TODO: Use encrypted blob and just save it instead decrypting and encrypting again
		n, err := repo.LoadBlob(ctx, entry.Type, entry.ID, buf)

		if err != nil {
			return errors.Wrap(err, "LoadBlob")
		}
		if n+crypto.Extension != int(entry.Length) {
			return errors.Errorf("error reading blob %v; length invalid. Want %d, got %d", entry.ID, entry.Length, n)
		}

		buf = buf[:n]
		_, err = repo.SaveBlob(ctx, entry.Type, buf, entry.ID)
		if err != nil {
			return err
		}
		debug.Log("  saved blob %v", entry.ID)
	}

	if p != nil {
		p.Report(restic.Stat{Blobs: 1})
	}
	return nil
}

func ChangePacksInIndex(opts CleanupPacksOptions, gopts GlobalOptions, repo restic.Repository, repackIndex *repository.Index) error {
	return ModifyIndex(opts.DryRun, gopts, repo, func(pb restic.PackedBlob) (changed bool, pbnew restic.PackedBlob) {
		pbs, found := repackIndex.Lookup(pb.ID, pb.Type)
		if found {
			changed = true
			pbnew = pbs[0]
		}
		return
	})
}
