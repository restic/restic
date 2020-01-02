package main

import (
	"context"

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
	DryRun        bool
	UnusedPercent int8
	UnusedSpace   int64
	UsedSpace     int64
}

var cleanupPacksOptions CleanupPacksOptions

func init() {
	cmdRoot.AddCommand(cmdCleanupPacks)

	f := cmdCleanupPacks.Flags()
	f.BoolVarP(&cleanupPacksOptions.DryRun, "dry-run", "n", false, "do not delete anything, just print what would be done")
	f.Int8Var(&cleanupPacksOptions.UnusedPercent, "unused-percent", -1, "if set, repack packs with more than given % unused space")
	f.Int64Var(&cleanupPacksOptions.UnusedSpace, "unused-space", -1, "if set, repack packs with more than given bytes unused space")
	f.Int64Var(&cleanupPacksOptions.UsedSpace, "used-space", -1, "if set, repack packs with less than given bytes used space")
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

func CleanupPacks(opts CleanupPacksOptions, gopts GlobalOptions, repo restic.Repository) error {

	ctx := gopts.ctx

	Verbosef("find packs in index and calculate used size...\n")
	packLength := make(map[restic.ID]uint)
	for blob := range repo.Index().Each(ctx) {
		if _, ok := packLength[blob.PackID]; !ok {
			// Start with 4 bytes overhead per pack (len of header)
			packLength[blob.PackID] = 4
		}
		// overhead per blob is the entry Size of the header
		// + crypto overhead
		packLength[blob.PackID] += blob.Length + pack.EntrySize + crypto.Extension
	}

	Verbosef("repack and collect packs for deletion\n")
	removePacks := restic.NewIDSet()
	repackPacks := restic.NewIDSet()
	removeBytes := uint64(0)
	repackBytes := uint64(0)
	repackFreeBytes := uint64(0)
	repackSmall := int(0)

	// TODO: Add parallel processing
	err := repo.List(ctx, restic.DataFile, func(id restic.ID, i64size int64) error {
		length, ok := packLength[id]
		usedSize := int64(length)
		packSize := i64size
		unusedSize := packSize - usedSize
		unusedPercent := int8((unusedSize * 100) / packSize)

		switch {
		case !ok:
			// Pack not in index! => remove!
			removePacks.Insert(id)
			removeBytes += uint64(packSize)
		case opts.UnusedPercent > 0 && unusedPercent > opts.UnusedPercent,
			opts.UnusedSpace > 0 && unusedSize > opts.UnusedSpace:
			repackPacks.Insert(id)
			repackBytes += uint64(packSize)
			repackFreeBytes += uint64(unusedSize)
		case opts.UsedSpace > 0 && usedSize < opts.UsedSpace:
			repackPacks.Insert(id)
			repackBytes += uint64(packSize)
			repackSmall++
		}
		return nil
	})
	if err != nil {
		return err
	}

	Verbosef("found %d unused packs\n", len(removePacks))
	Verbosef("deleting unused packs will free about %s\n\n", formatBytes(removeBytes))

	Verbosef("found %d partly used packs + %d small packs = total %d packs for repacking with %s\n",
		len(repackPacks)-repackSmall, repackSmall, len(repackPacks), formatBytes(repackBytes))
	Verbosef("repacking packs will free about %s\n", formatBytes(repackFreeBytes))

	Verbosef("cleanup-packs will totally free about %s\n", formatBytes(removeBytes+repackFreeBytes))

	if len(repackPacks) > 0 {
		Verbosef("repacking packs...\n")

		for id := range repackPacks {
			if !opts.DryRun {
				err := repack(ctx, repo, id, nil)
				if err != nil {
					return err
				}
			} else {
				if !gopts.JSON {
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

	// TODO: Add parallel processing
	if len(removePacks) != 0 {
		Verbosef("deleting packs (unused and repacked)\n")
		bar := newProgressMax(!gopts.Quiet, uint64(len(removePacks)), "packs deleted")
		bar.Start()
		for packID := range removePacks {
			if !opts.DryRun {
				h := restic.Handle{Type: restic.DataFile, Name: packID.String()}
				err = repo.Backend().Remove(ctx, h)
				if err != nil {
					Warnf("unable to remove file %v from the repository\n", packID.Str())
				}
				if !gopts.JSON {
					Verbosef("pack %v was removed.\n", packID.Str())
				}
			} else {
				if !gopts.JSON {
					Verbosef("would have removed pack %v.\n", packID.Str())
				}
			}
			bar.Report(restic.Stat{Blobs: 1})
		}
		bar.Done()
	}

	Verbosef("done\n")
	return nil
}

func repack(ctx context.Context, repo restic.Repository, packID restic.ID, p *restic.Progress) (err error) {

	// load the complete pack into a temp file
	h := restic.Handle{Type: restic.DataFile, Name: packID.String()}

	// TODO: Use index instead of pack header
	tempfile, hash, packLength, err := repository.DownloadAndHash(ctx, repo.Backend(), h)
	if err != nil {
		return errors.Wrap(err, "Repack")
	}

	debug.Log("pack %v loaded (%d bytes), hash %v", packID, packLength, hash)

	if !packID.Equal(hash) {
		return errors.Errorf("hash does not match id: want %v, got %v", packID, hash)
	}

	_, err = tempfile.Seek(0, 0)
	if err != nil {
		return errors.Wrap(err, "Seek")
	}

	blobs, err := pack.List(repo.Key(), tempfile, packLength)
	if err != nil {
		return err
	}

	debug.Log("processing pack %v, blobs: %v", packID, len(blobs))
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
			Verbosef("replace packed blob %v by %v\n", pb, pbs[0])
			changed = true
			pbnew = pbs[0]
		}
		return
	})
}
