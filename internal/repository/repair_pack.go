package repository

import (
	"bytes"
	"context"
	"io"
	"iter"
	"maps"
	"os"
	"slices"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository/pack"
	"github.com/restic/restic/internal/restic"
)

func makeShortIDs(ids iter.Seq[restic.ID]) (result map[string]restic.ID) {
	result = make(map[string]restic.ID)
	for id := range ids {
		result[id.Str()] = id
	}

	return result
}

// RepairPacks fixes broken / non-existing packfiles (but known to the master index)
// there are two input parameters possible:
// args = []string which is used in the mainline code and in the integration tests
// ids = restic.IDSet for internal/repository/repair_pack_test.
// If ids is empty, the args slice is used.
func RepairPacks(ctx context.Context, repo *Repository, snapshotLister restic.Lister, args []string, ids restic.IDSet, printer restic.Printer,
) error {
	printer.P("salvaging intact data from specified pack files")

	// validate 'ids': it is either a valid packfile or it appears in the master index
	packsFromIndex, err := pack.Size(ctx, repo, false)
	if err != nil {
		return err
	}
	shortIDs := makeShortIDs(maps.Keys(packsFromIndex))

	if len(ids) == 0 {
		for _, arg := range args {
			id, err := restic.Find(ctx, snapshotLister, restic.PackFile, arg)
			idFromIndex, ok := shortIDs[arg]
			if err != nil && !ok {
				return errors.Fatalf("%q is not a valid packfile", arg)
			}
			if ok {
				id = idFromIndex
			}
			// id is either a valid packfile or has been found in the master index
			ids.Insert(id)
		}
	}
	if len(ids) == 0 {
		return errors.Fatal("no ids specified")
	}

	printer.P("saving backup copies of pack files to current folder")
	for id := range ids {
		buf, err := repo.LoadRaw(ctx, restic.PackFile, id)
		// corrupted data is fine
		if err == nil || buf != nil {
			f, err := os.OpenFile("pack-"+id.String(), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, bytes.NewReader(buf)); err != nil {
				_ = f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
	}

	packToBlobs, err := resolveBlobsForPacks(ctx, repo, ids)
	if err != nil {
		return err
	}

	bar := printer.NewCounter("pack files")
	bar.SetMax(uint64(len(ids)))
	defer bar.Done()
	err = repo.WithBlobUploader(ctx, func(ctx context.Context, uploader restic.BlobSaverWithAsync) error {
		// examine all data the indexes have for the pack file
		for b := range repo.listPacksFromIndex(ctx, ids) {
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

func resolveBlobsForPacks(ctx context.Context, repo *Repository, ids restic.IDSet) (map[restic.ID]pack.Blobs, error) {
	packToBlobs := make(map[restic.ID]pack.Blobs)

	err := repo.List(ctx, restic.PackFile, func(id restic.ID, size int64) error {
		if ids.Has(id) {
			blobs, err := repo.listPack(ctx, id, size)
			if err != nil {
				// ignore errors for broken pack files to be able to salvage as much as possible
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

func reuploadBlobsFromPack(ctx context.Context, repo *Repository, packID restic.ID, blobs pack.Blobs, printer restic.Printer, uploader restic.BlobSaverWithAsync,
) error {
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
