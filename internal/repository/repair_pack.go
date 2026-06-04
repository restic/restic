package repository

import (
	"context"
	"errors"
	"io"
	"slices"

	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/progress"
)

func RepairPacks(ctx context.Context, repo *Repository, ids restic.IDSet, printer progress.Printer) error {
	printer.P("salvaging intact data from specified pack files")
	bar := printer.NewCounter("pack files")
	bar.SetMax(uint64(len(ids)))
	defer bar.Done()

	packToBlobs, err := resolveBlobsForPacks(ctx, repo, ids)
	if err != nil {
		return err
	}

	err = repo.WithBlobUploader(ctx, func(ctx context.Context, uploader restic.BlobSaverWithAsync) error {
		// examine all data the indexes have for the pack file
		for b := range repo.ListPacksFromIndex(ctx, ids) {
			indexBlobs := b.Blobs
			err := reuploadBlobsFromPack(ctx, repo, b.PackID, indexBlobs, printer, uploader)
			if err != nil {
				return err
			}

			indexBlobs.Sort()
			packBlobs := packToBlobs[b.PackID]
			packBlobs.Sort()
			if packBlobs != nil && !slices.Equal(indexBlobs, packBlobs) {
				// handle case where the index entry is broken or incomplete.
				// this can result in duplicate blobs, which can be cleaned up by running prune.
				printer.E("repairing incomplete index entry for pack %v", b.PackID)
				err := reuploadBlobsFromPack(ctx, repo, b.PackID, packBlobs, printer, uploader)
				if err != nil {
					return err
				}
			}
			if len(indexBlobs) == 0 && len(packBlobs) == 0 {
				printer.E("no blobs found for pack %v", b.PackID)
			}

			bar.Add(1)
		}
		return nil
	})
	if err != nil {
		return err
	}
	bar.Done()

	// remove salvaged packs from index
	err = rewriteIndexFiles(ctx, repo, ids, nil, nil, printer)
	if err != nil {
		return err
	}

	// cleanup
	printer.P("removing salvaged pack files")
	// if we fail to delete the damaged pack files, then prune will remove them later on
	bar = printer.NewCounter("files deleted")
	_ = restic.ParallelRemove(ctx, &internalRepository{repo}, ids, restic.PackFile, nil, bar)
	bar.Done()

	return nil
}

func resolveBlobsForPacks(ctx context.Context, repo *Repository, ids restic.IDSet) (map[restic.ID]restic.Blobs, error) {
	packToBlobs := make(map[restic.ID]restic.Blobs)

	err := repo.List(ctx, restic.PackFile, func(id restic.ID, size int64) error {
		if ids.Has(id) {
			blobs, err := repo.ListPack(ctx, id, size)
			if err != nil {
				return nil
			}
			packToBlobs[id] = blobs
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return packToBlobs, nil
}

func reuploadBlobsFromPack(ctx context.Context, repo *Repository, packID restic.ID, blobs restic.Blobs, printer progress.Printer, uploader restic.BlobSaverWithAsync) error {
	err := repo.loadBlobsFromPack(ctx, packID, blobs, func(blob restic.BlobHandle, buf []byte, err error) error {
		if err != nil {
			printer.E("failed to load blob %v: %v", blob.ID, err)
			return nil
		}
		id, _, _, err := uploader.SaveBlob(ctx, blob.Type, buf, restic.ID{}, true)
		if err == nil && !id.Equal(blob.ID) {
			panic("pack id mismatch during upload")
		}
		return err
	})
	// ignore truncated file parts
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return err
	}
	return nil
}
