package index

import (
	"testing"

	"github.com/restic/restic/internal/repository/pack"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func TestIndexOversized(t *testing.T) {
	idx := NewIndex()

	// Add blobs up to indexMaxBlobs + pack.MaxHeaderEntries - 1
	packID := idx.addToPacks(restic.NewRandomID())
	id := restic.NewRandomID()
	for i := uint(0); i < indexMaxBlobs+pack.MaxHeaderEntries-1; i++ {
		// Directly modify ID to avoid benchmarking NewRandomID
		id[0] = byte(i)
		id[1] = byte(i >> 8)
		id[2] = byte(i >> 16)
		id[3] = byte(i >> 24)

		idx.store(packID, pack.Blob{
			BlobHandle: restic.BlobHandle{
				Type: restic.DataBlob,
				ID:   id,
			},
			Length: 100,
			Offset: uint(i) * 100,
		})
	}

	rtest.Assert(t, !Oversized(idx), "index should not be considered oversized")

	// Add one more blob to exceed the limit
	idx.store(packID, pack.Blob{
		BlobHandle: restic.BlobHandle{
			Type: restic.DataBlob,
			ID:   restic.NewRandomID(),
		},
		Length: 100,
		Offset: uint(indexMaxBlobs+pack.MaxHeaderEntries) * 100,
	})

	rtest.Assert(t, Oversized(idx), "index should be considered oversized")
}
