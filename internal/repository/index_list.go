package repository

import (
	"context"
	"iter"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository/index"
	"github.com/restic/restic/internal/restic"
)

// IndexBlob is one blob handle from an on-disk index file, or an error from loading/decoding
// that file.
type IndexBlob struct {
	Handle restic.BlobHandle
	Error  error
}

// AllIndexBlobs streams blob handles from each index file without building a master index.
func AllIndexBlobs(ctx context.Context, lister restic.Lister, loader restic.LoaderUnpacked) iter.Seq[IndexBlob] {
	return func(yield func(IndexBlob) bool) {
		stopIteration := errors.New("stop index blob iteration")
		err := index.ForAllIndexes(ctx, lister, loader, func(_ restic.ID, idx *index.Index, err error) error {
			if err != nil {
				return err
			}
			for blob := range idx.Values() {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if !yield(IndexBlob{Handle: blob.Handle()}) {
					return stopIteration
				}
			}
			return nil
		})
		if err != nil && !errors.Is(err, stopIteration) {
			yield(IndexBlob{Error: err})
		}
	}
}
