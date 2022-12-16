package repository

import (
	"context"
	"testing"

	"github.com/restic/restic/internal/backend/mem"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/sync/errgroup"
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
		repo := TestRepositoryWithBackend(t, mem.New(), 2)

		var wg errgroup.Group
		repo.StartPackUploader(context.TODO(), &wg)

		_, _, _, err := repo.SaveBlob(context.TODO(), restic.DataBlob, blob, id, false)
		if err != nil {
			t.Fatal(err)
		}
		err = repo.Flush(context.TODO())
		if err != nil {
			t.Fatal(err)
		}

		buf, err := repo.LoadBlob(context.TODO(), restic.DataBlob, id, make([]byte, buflen))
		if err != nil {
			t.Fatal(err)
		}
		if restic.Hash(buf) != id {
			t.Fatal("mismatch")
		}
	})
}
