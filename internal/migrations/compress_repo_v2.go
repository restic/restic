package migrations

import (
	"context"
	"fmt"

	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
)

func init() {
	register(&CompressRepoV2{})
}

type CompressRepoV2 struct{}

func (*CompressRepoV2) Name() string {
	return "compress_all_data"
}

func (*CompressRepoV2) Desc() string {
	return "compress all data in the repo"
}

func (*CompressRepoV2) Check(ctx context.Context, repo restic.Repository) (bool, error) {
	// only do very fast checks on the version here, we don't want the list of
	// available migrations to take long to load
	if repo.Config().Version < 2 {
		return false, nil
	}

	return true, nil
}

// Apply requires that the repository must be already locked exclusively, this
// is done by the caller, so we can just go ahead, rewrite the packs as they
// are, remove the packs and rebuild the index.
func (*CompressRepoV2) Apply(ctx context.Context, repo restic.Repository) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	err := repo.LoadIndex(ctx)
	if err != nil {
		return fmt.Errorf("index load failed: %w", err)
	}

	packsWithUncompressedData := restic.NewIDSet()
	keepBlobs := restic.NewBlobSet()

	for blob := range repo.Index().Each(ctx) {
		keepBlobs.Insert(blob.BlobHandle)

		if blob.UncompressedLength != 0 {
			// blob is already compressed, ignore
			continue

		}

		// remember pack ID
		packsWithUncompressedData.Insert(blob.PackID)
	}

	if len(packsWithUncompressedData) == 0 {
		// nothing to do
		return nil
	}

	// don't upload new indexes until we're done
	repo.(*repository.Repository).DisableAutoIndexUpdate()
	obsoletePacks, err := repository.Repack(ctx, repo, repo, packsWithUncompressedData, keepBlobs, nil)
	if err != nil {
		return fmt.Errorf("repack failed: %w", err)
	}

	if len(obsoletePacks) != len(packsWithUncompressedData) {
		return fmt.Errorf("Repack() return other packs, %d != %d", len(obsoletePacks), len(packsWithUncompressedData))
	}

	// build new index
	idx := repo.Index().(*repository.MasterIndex)
	obsoleteIndexes, err := idx.Save(ctx, repo, obsoletePacks, nil, nil)
	if err != nil {
		return fmt.Errorf("saving new index failed: %w", err)
	}

	// remove data
	for id := range obsoleteIndexes {
		err = repo.Backend().Remove(ctx, restic.Handle{Name: id.String(), Type: restic.IndexFile})
		if err != nil {
			return fmt.Errorf("remove file failed: %w", err)
		}
	}

	for id := range obsoletePacks {
		err = repo.Backend().Remove(ctx, restic.Handle{Name: id.String(), Type: restic.PackFile})
		if err != nil {
			return fmt.Errorf("remove file failed: %w", err)
		}
	}

	return nil
}
