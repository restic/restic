package index

import (
	"context"
	"testing"

	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
)

type noopSaver struct{}

func (n *noopSaver) Connections() uint {
	return 2
}
func (n *noopSaver) SaveUnpacked(ctx context.Context, t restic.FileType, buf []byte) (restic.ID, error) {
	return restic.Hash(buf), nil
}

func makeFakePackedBlob() (restic.BlobHandle, restic.PackedBlob) {
	bh := restic.NewRandomBlobHandle()
	blob := restic.PackedBlob{
		PackID: restic.NewRandomID(),
		Blob: restic.Blob{
			BlobHandle: bh,
			Length:     uint(crypto.CiphertextLength(10)),
			Offset:     0,
		},
	}
	return bh, blob
}

func TestAssociatedSet(t *testing.T) {
	bh, blob := makeFakePackedBlob()

	mi := NewMasterIndex()
	mi.StorePack(blob.PackID, []restic.Blob{blob.Blob})
	test.OK(t, mi.SaveIndex(context.TODO(), &noopSaver{}))

	bs := NewAssociatedSet[uint8](mi)
	test.Equals(t, bs.Len(), 0)
	test.Equals(t, bs.List(), restic.BlobHandles{})

	// check non existent
	test.Equals(t, bs.Has(bh), false)
	_, ok := bs.Get(bh)
	test.Equals(t, false, ok)

	// test insert
	bs.Insert(bh)
	test.Equals(t, bs.Has(bh), true)
	test.Equals(t, bs.Len(), 1)
	test.Equals(t, bs.List(), restic.BlobHandles{bh})
	test.Equals(t, 0, len(bs.overflow))

	// test set
	bs.Set(bh, 42)
	test.Equals(t, bs.Has(bh), true)
	test.Equals(t, bs.Len(), 1)
	val, ok := bs.Get(bh)
	test.Equals(t, true, ok)
	test.Equals(t, uint8(42), val)

	s := bs.String()
	test.Assert(t, len(s) > 10, "invalid string: %v", s)

	// test remove
	bs.Delete(bh)
	test.Equals(t, bs.Len(), 0)
	test.Equals(t, bs.Has(bh), false)
	test.Equals(t, bs.List(), restic.BlobHandles{})

	test.Equals(t, "{}", bs.String())

	// test set
	bs.Set(bh, 43)
	test.Equals(t, bs.Has(bh), true)
	test.Equals(t, bs.Len(), 1)
	val, ok = bs.Get(bh)
	test.Equals(t, true, ok)
	test.Equals(t, uint8(43), val)
	test.Equals(t, 0, len(bs.overflow))
	// test update
	bs.Set(bh, 44)
	val, ok = bs.Get(bh)
	test.Equals(t, true, ok)
	test.Equals(t, uint8(44), val)
	test.Equals(t, 0, len(bs.overflow))

	// test overflow blob
	of := restic.NewRandomBlobHandle()
	test.Equals(t, false, bs.Has(of))
	// set
	bs.Set(of, 7)
	test.Equals(t, 1, len(bs.overflow))
	test.Equals(t, bs.Len(), 2)
	// get
	val, ok = bs.Get(of)
	test.Equals(t, true, ok)
	test.Equals(t, uint8(7), val)
	test.Equals(t, bs.List(), restic.BlobHandles{of, bh})
	// update
	bs.Set(of, 8)
	val, ok = bs.Get(of)
	test.Equals(t, true, ok)
	test.Equals(t, uint8(8), val)
	test.Equals(t, 1, len(bs.overflow))
	// delete
	bs.Delete(of)
	test.Equals(t, bs.Len(), 1)
	test.Equals(t, bs.Has(of), false)
	test.Equals(t, bs.List(), restic.BlobHandles{bh})
	test.Equals(t, 0, len(bs.overflow))
}

func TestAssociatedSetWithExtendedIndex(t *testing.T) {
	_, blob := makeFakePackedBlob()

	mi := NewMasterIndex()
	mi.StorePack(blob.PackID, []restic.Blob{blob.Blob})
	test.OK(t, mi.SaveIndex(context.TODO(), &noopSaver{}))

	bs := NewAssociatedSet[uint8](mi)

	// add new blobs to index after building the set
	of, blob2 := makeFakePackedBlob()
	mi.StorePack(blob2.PackID, []restic.Blob{blob2.Blob})
	test.OK(t, mi.SaveIndex(context.TODO(), &noopSaver{}))

	// non-existent
	test.Equals(t, false, bs.Has(of))
	// set
	bs.Set(of, 5)
	test.Equals(t, 1, len(bs.overflow))
	test.Equals(t, bs.Len(), 1)
	// get
	val, ok := bs.Get(of)
	test.Equals(t, true, ok)
	test.Equals(t, uint8(5), val)
	test.Equals(t, bs.List(), restic.BlobHandles{of})
	// update
	bs.Set(of, 8)
	val, ok = bs.Get(of)
	test.Equals(t, true, ok)
	test.Equals(t, uint8(8), val)
	test.Equals(t, 1, len(bs.overflow))
	// delete
	bs.Delete(of)
	test.Equals(t, bs.Len(), 0)
	test.Equals(t, bs.Has(of), false)
	test.Equals(t, bs.List(), restic.BlobHandles{})
	test.Equals(t, 0, len(bs.overflow))
}
