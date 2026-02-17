package repository

import (
	"context"
	"testing"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

// Test saving a blob and loading it again, with varying buffer sizes.
// Also a regression test for #3783.
func FuzzSaveLoadBlob(f *testing.F) {
	f.Fuzz(func(t *testing.T, blob []byte, buflen uint) {
		if buflen > 64<<20 {
			// Don't allocate enormous buffers. We're not testing the allocator.
			t.Skip()
		}

		id := restic.Hash(blob)
		repo, _, _ := TestRepositoryWithVersion(t, 2)

		rtest.OK(t, repo.WithBlobUploader(context.TODO(), func(ctx context.Context, uploader restic.BlobSaverWithAsync) error {
			_, _, _, err := uploader.SaveBlob(ctx, restic.DataBlob, blob, id, false)
			return err
		}))

		buf, err := repo.LoadBlob(context.TODO(), restic.DataBlob, id, make([]byte, buflen))
		if err != nil {
			t.Fatal(err)
		}
		if restic.Hash(buf) != id {
			t.Fatal("mismatch")
		}
	})
}
