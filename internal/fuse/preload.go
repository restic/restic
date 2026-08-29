//go:build darwin || freebsd || linux

package fuse

import (
	"bytes"
	"context"
	"fmt"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/sync/errgroup"
)

// PreloadTreeMetadata enumerates tree blobs from the loaded index and loads them
// grouped by pack. It intentionally does not load data blobs containing file
// contents.
func (r *Root) PreloadTreeMetadata(ctx context.Context, p restic.Counter) error {
	treePacks, treeCount, err := r.treeBlobsByPack(ctx)
	if err != nil {
		return err
	}

	p.SetMax(uint64(treeCount))

	wg, ctx := errgroup.WithContext(ctx)
	wg.SetLimit(max(1, int(r.repo.Connections())))

	for packID, blobs := range treePacks {
		wg.Go(func() error {
			err := r.repo.LoadBlobsFromPack(ctx, packID, blobs, func(blob restic.BlobHandle, buf []byte, err error) error {
				if err != nil {
					return fmt.Errorf("loading tree %v: %w", blob.ID.Str(), err)
				}

				nodes, err := data.NewTreeNodeIterator(bytes.NewReader(buf))
				if err != nil {
					return fmt.Errorf("loading tree %v: %w", blob.ID.Str(), err)
				}

				for item := range nodes {
					if item.Error != nil {
						return fmt.Errorf("loading tree %v: %w", blob.ID.Str(), item.Error)
					}
					if ctx.Err() != nil {
						return ctx.Err()
					}
				}

				p.Add(1)
				return nil
			})
			if err != nil {
				return fmt.Errorf("loading trees from pack %v: %w", packID.Str(), err)
			}
			return nil
		})
	}

	return wg.Wait()
}

func (r *Root) treeBlobsByPack(ctx context.Context) (map[restic.ID][]restic.BlobHandle, int, error) {
	treePacks := make(map[restic.ID][]restic.BlobHandle)
	seen := restic.NewBlobSet()

	err := r.repo.ListBlobs(ctx, func(blob restic.PackBlob) {
		handle := blob.Handle()
		if handle.Type != restic.TreeBlob || seen.Has(handle) {
			return
		}

		seen.Insert(handle)
		packID := blob.PackID()
		treePacks[packID] = append(treePacks[packID], handle)
	})
	if err != nil {
		return nil, 0, err
	}

	return treePacks, seen.Len(), nil
}
