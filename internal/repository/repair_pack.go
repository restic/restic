package repository

import (
	"context"
	"errors"
	"io"

	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/progress"
	"golang.org/x/sync/errgroup"
)

func RepairPacks(ctx context.Context, repo *Repository, ids restic.IDSet, printer progress.Printer) error {
	wg, wgCtx := errgroup.WithContext(ctx)
	repo.StartPackUploader(wgCtx, wg)

	printer.P("salvaging intact data from specified pack files")
	bar := printer.NewCounter("pack files")
	bar.SetMax(uint64(len(ids)))
	defer bar.Done()

	wg.Go(func() error {
		// examine all data the indexes have for the pack file
		for b := range repo.ListPacksFromIndex(wgCtx, ids) {
			blobs := b.Blobs
			if len(blobs) == 0 {
				printer.E("no blobs found for pack %v", b.PackID)
				bar.Add(1)
				continue
			}

			err := repo.LoadBlobsFromPack(wgCtx, b.PackID, blobs, func(blob restic.BlobHandle, buf []byte, err error) error {
				if err != nil {
					printer.E("failed to load blob %v: %v", blob.ID, err)
					return nil
				}
				id, _, _, err := repo.SaveBlob(wgCtx, blob.Type, buf, restic.ID{}, true)
				if !id.Equal(blob.ID) {
					panic("pack id mismatch during upload")
				}
				return err
			})
			// ignore truncated file parts
			if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
				return err
			}
			bar.Add(1)
		}
		return repo.Flush(wgCtx)
	})

	err := wg.Wait()
	bar.Done()
	if err != nil {
		return err
	}

	// remove salvaged packs from index
	err = rewriteIndexFiles(ctx, repo, ids, nil, nil, printer)
	if err != nil {
		return err
	}

	// cleanup
	printer.P("removing salvaged pack files")
	// if we fail to delete the damaged pack files, then prune will remove them later on
	bar = printer.NewCounter("files deleted")
	_ = restic.ParallelRemove(ctx, repo, ids, restic.PackFile, nil, bar)
	bar.Done()

	return nil
}
