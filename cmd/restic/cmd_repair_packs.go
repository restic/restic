package main

import (
	"context"
	"io"
	"os"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

var cmdRepairPacks = &cobra.Command{
	Use:   "packs [packIDs...]",
	Short: "Salvage damaged pack files",
	Long: `
WARNING: The CLI for this command is experimental and will likely change in the future!

The "repair packs" command extracts intact blobs from the specified pack files, rebuilds
the index to remove the damaged pack files and removes the pack files from the repository.

EXIT STATUS
===========

Exit status is 0 if the command was successful, and non-zero if there was any error.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRepairPacks(cmd.Context(), globalOptions, args)
	},
}

func init() {
	cmdRepair.AddCommand(cmdRepairPacks)
}

func runRepairPacks(ctx context.Context, gopts GlobalOptions, args []string) error {
	// FIXME discuss and add proper feature flag mechanism
	flag, _ := os.LookupEnv("RESTIC_FEATURES")
	if flag != "repair-packs-v1" {
		return errors.Fatal("This command is experimental and may change/be removed without notice between restic versions. " +
			"Set the environment variable 'RESTIC_FEATURES=repair-packs-v1' to enable it.")
	}

	ids := restic.NewIDSet()
	for _, arg := range args {
		id, err := restic.ParseID(arg)
		if err != nil {
			return err
		}
		ids.Insert(id)
	}
	if len(ids) == 0 {
		return errors.Fatal("no ids specified")
	}

	repo, err := OpenRepository(ctx, gopts)
	if err != nil {
		return err
	}

	lock, ctx, err := lockRepoExclusive(ctx, repo, gopts.RetryLock, gopts.JSON)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	return repairPacks(ctx, gopts, repo, ids)
}

func repairPacks(ctx context.Context, gopts GlobalOptions, repo *repository.Repository, ids restic.IDSet) error {
	bar := newIndexProgress(gopts.Quiet, gopts.JSON)
	err := repo.LoadIndex(ctx, bar)
	if err != nil {
		return errors.Fatalf("%s", err)
	}

	Warnf("saving backup copies of pack files in current folder\n")
	for id := range ids {
		f, err := os.OpenFile("pack-"+id.String(), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
		if err != nil {
			return errors.Fatalf("%s", err)
		}

		err = repo.Backend().Load(ctx, restic.Handle{Type: restic.PackFile, Name: id.String()}, 0, 0, func(rd io.Reader) error {
			_, err := f.Seek(0, 0)
			if err != nil {
				return err
			}
			_, err = io.Copy(f, rd)
			return err
		})
		if err != nil {
			return errors.Fatalf("%s", err)
		}
	}

	wg, wgCtx := errgroup.WithContext(ctx)
	repo.StartPackUploader(wgCtx, wg)
	repo.DisableAutoIndexUpdate()

	Warnf("salvaging intact data from specified pack files\n")
	bar = newProgressMax(!gopts.Quiet, uint64(len(ids)), "pack files")
	defer bar.Done()

	wg.Go(func() error {
		// examine all data the indexes have for the pack file
		for b := range repo.Index().ListPacks(wgCtx, ids) {
			blobs := b.Blobs
			if len(blobs) == 0 {
				Warnf("no blobs found for pack %v\n", b.PackID)
				bar.Add(1)
				continue
			}

			err = repository.StreamPack(wgCtx, repo.Backend().Load, repo.Key(), b.PackID, blobs, func(blob restic.BlobHandle, buf []byte, err error) error {
				if err != nil {
					// Fallback path
					buf, err = repo.LoadBlob(wgCtx, blob.Type, blob.ID, nil)
					if err != nil {
						Warnf("failed to load blob %v: %v\n", blob.ID, err)
						return nil
					}
				}
				id, _, _, err := repo.SaveBlob(wgCtx, blob.Type, buf, restic.ID{}, true)
				if !id.Equal(blob.ID) {
					panic("pack id mismatch during upload")
				}
				return err
			})
			if err != nil {
				return err
			}
			bar.Add(1)
		}
		return repo.Flush(wgCtx)
	})

	if err := wg.Wait(); err != nil {
		return errors.Fatalf("%s", err)
	}
	bar.Done()

	// remove salvaged packs from index
	err = rebuildIndexFiles(ctx, gopts, repo, ids, nil)
	if err != nil {
		return errors.Fatalf("%s", err)
	}

	// cleanup
	Warnf("removing salvaged pack files\n")
	DeleteFiles(ctx, gopts, repo, ids, restic.PackFile)

	Warnf("\nUse `restic repair snapshots --forget` to remove the corrupted data blobs from all snapshots\n")
	return nil
}
