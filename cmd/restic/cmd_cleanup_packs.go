package main

import (
	"context"
	"fmt"
	"os"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/pack"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"

	"github.com/spf13/cobra"
)

var cmdCleanupPacks = &cobra.Command{
	Use:   "cleanup-packs [flags]",
	Short: "Remove packs not in index",
	Long: `
The "cleanup-packs" command removes packs
that are not contained in any index files.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCleanupPacks(cleanupPacksOptions, globalOptions)
	},
}

// CleanupIndexOptions collects all options for the cleanup-index command.
type CleanupPacksOptions struct {
	DryRun bool
}

var cleanupPacksOptions CleanupPacksOptions

func init() {
	cmdRoot.AddCommand(cmdCleanupPacks)

	f := cmdCleanupPacks.Flags()
	f.BoolVarP(&cleanupPacksOptions.DryRun, "dry-run", "n", false, "do not delete anything, just print what would be done")
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

	Verbosef("find blobs in index\n")
	packLength := make(map[restic.ID]uint64)
	for blob := range repo.Index().Each(ctx) {
		if _, ok := packLength[blob.PackID]; !ok {
			// Start with 4 bytes overhead per pack (len of header)
			packLength[blob.PackID] = 4
		}
		// overhead per blob is 16 bytes IV + 16 bytes MAc + (1+4+32) bytes in header
		// => total overhead per blob: 69 Bytes
		packLength[blob.PackID] += uint64(blob.Length) + 69
	}

	Verbosef("repack and collect packs for deletion\n")
	removePacks := restic.NewIDSet()
	removeBytes := uint64(0)
	repackBytes := uint64(0)
	repackedPacks := 0

	// TODO: Add parallel processing
	err := repo.List(ctx, restic.DataFile, func(id restic.ID, size int64) error {
		length, ok := packLength[id]
		uintSize := uint64(size)
		if !ok {
			// Pack not in index! => remove!
			removePacks.Insert(id)
			removeBytes += uintSize
		} else {
			// TODO: add threshold
			if uintSize > length {
				Verbosef("size of pack %s: %s, / used: %s\n", id.String(), formatBytes(uintSize), formatBytes(length))
				if !opts.DryRun {
					err := repack(ctx, repo, id, nil)
					if err != nil {
						return err
					}
				}
				repackBytes += length
				repackedPacks++
				// Also remove Pack at the end!
				removePacks.Insert(id)
				removeBytes += uint64(size)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	Verbosef("repacked %d packs with %s\n", repackedPacks, formatBytes(repackBytes))

	if repackedPacks > 0 {
		Verbosef("updating index files...\n")

		notFinalIdx := (repo.Index()).(*repository.MasterIndex).NotFinalIndexes()
		if len(notFinalIdx) != 1 {
			return errors.Fatal("should only have one unfinalized index!!")
		}
		repackIndex := notFinalIdx[0]

		indexlist := restic.NewIDSet()
		// TODO: Add parallel processing
		err = repo.List(ctx, restic.IndexFile, func(id restic.ID, size int64) error {
			indexlist.Insert(id)
			return nil
		})
		if err != nil {
			return err
		}

		Verbosef("check %d files and change if neccessary\n", len(indexlist))
		bar := newProgressMax(!gopts.Quiet, uint64(len(indexlist)), "index files processed")
		bar.Start()
		// TODO: Add parallel processing
		for id := range indexlist {
			idxNew := repository.NewIndex()
			err := idxNew.AddToSupersedes(id)
			if err != nil {
				return err
			}

			idx, err := repository.LoadIndex(ctx, repo, id)
			if err != nil {
				return err
			}

			changed := false
			for pb := range idx.Each(ctx) {
				pbs, found := repackIndex.Lookup(pb.ID, pb.Type)
				if found {
					idxNew.Store(pbs[0])
					changed = true
				} else {
					idxNew.Store(pb)
				}
			}
			if changed {
				if !opts.DryRun {
					newID, err := repository.SaveIndex(ctx, repo, idxNew)
					if err != nil {
						return err
					}
					h := restic.Handle{Type: restic.IndexFile, Name: id.String()}
					err = repo.Backend().Remove(ctx, h)
					if err != nil {
						Warnf("unable to remove index %v from the repository\n", id.Str())
					}
					if !gopts.JSON {
						Verbosef("index %v was removed. new index: %v\n", id.Str(), newID.Str())
					}
				} else {
					if !gopts.JSON {
						Verbosef("would have replaced index %v\n", id.Str())
					}
				}
			}
			bar.Report(restic.Stat{Files: 1})
		}
		bar.Done()
	}

	Verbosef("will now delete %d packs\n", len(removePacks))
	Verbosef("frees %s\n", formatBytes(removeBytes))

	// TODO: Add parallel processing
	if len(removePacks) != 0 {
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
		h := restic.BlobHandle{ID: entry.ID, Type: entry.Type}
		// if blob not in index, don't write it
		if !repo.Index().Has(entry.ID, entry.Type) {
			continue
		}

		debug.Log("  process blob %v", h)

		buf = buf[:]
		if uint(len(buf)) < entry.Length {
			buf = make([]byte, entry.Length)
		}
		buf = buf[:entry.Length]

		n, err := tempfile.ReadAt(buf, int64(entry.Offset))
		if err != nil {
			return errors.Wrap(err, "ReadAt")
		}

		if n != len(buf) {
			return errors.Errorf("read blob %v from %v: not enough bytes read, want %v, got %v",
				h, tempfile.Name(), len(buf), n)
		}

		nonce, ciphertext := buf[:repo.Key().NonceSize()], buf[repo.Key().NonceSize():]
		plaintext, err := repo.Key().Open(ciphertext[:0], nonce, ciphertext, nil)
		if err != nil {
			return err
		}

		id := restic.Hash(plaintext)
		if !id.Equal(entry.ID) {
			debug.Log("read blob %v/%v from %v: wrong data returned, hash is %v",
				h.Type, h.ID, tempfile.Name(), id)
			fmt.Fprintf(os.Stderr, "read blob %v from %v: wrong data returned, hash is %v",
				h, tempfile.Name(), id)
		}

		_, err = repo.SaveBlob(ctx, entry.Type, plaintext, entry.ID)
		if err != nil {
			return err
		}

		debug.Log("  saved blob %v", entry.ID)

	}

	if err = tempfile.Close(); err != nil {
		return errors.Wrap(err, "Close")
	}

	if err = fs.RemoveIfExists(tempfile.Name()); err != nil {
		return errors.Wrap(err, "Remove")
	}
	if p != nil {
		p.Report(restic.Stat{Blobs: 1})
	}
	return nil
}
