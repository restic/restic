package restic_test

import (
	"testing"

	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
)

func TestCountedBlobSet(t *testing.T) {
	bs := restic.NewCountedBlobSet()
	test.Equals(t, bs.Len(), 0)
	test.Equals(t, bs.List(), restic.BlobHandles{})

	bh := restic.NewRandomBlobHandle()
	// check non existant
	test.Equals(t, bs.Has(bh), false)

	// test insert
	bs.Insert(bh)
	test.Equals(t, bs.Has(bh), true)
	test.Equals(t, bs.Len(), 1)
	test.Equals(t, bs.List(), restic.BlobHandles{bh})

	// test remove
	bs.Delete(bh)
	test.Equals(t, bs.Len(), 0)
	test.Equals(t, bs.Has(bh), false)
	test.Equals(t, bs.List(), restic.BlobHandles{})

	bs = restic.NewCountedBlobSet(bh)
	test.Equals(t, bs.Len(), 1)
	test.Equals(t, bs.List(), restic.BlobHandles{bh})

	s := bs.String()
	test.Assert(t, len(s) > 10, "invalid string: %v", s)
}
