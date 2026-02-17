package index

import (
	"context"
	"slices"
	"testing"

	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
)

type noopSaver struct{}

func (n *noopSaver) Connections() uint {
	return 2
}
func (n *noopSaver) SaveUnpacked(_ context.Context, _ restic.FileType, buf []byte) (restic.ID, error) {
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

func list(bs *AssociatedSet[uint8]) restic.BlobHandles {
	return restic.BlobHandles(slices.Collect(bs.Keys()))
}

func TestAssociatedSet(t *testing.T) {
	bh, blob := makeFakePackedBlob()

	mi := NewMasterIndex()
	test.OK(t, mi.StorePack(context.TODO(), blob.PackID, []restic.Blob{blob.Blob}, &noopSaver{}))
	test.OK(t, mi.Flush(context.TODO(), &noopSaver{}))

	bs := NewAssociatedSet[uint8](mi)
	test.Equals(t, bs.Len(), 0)
	test.Equals(t, list(bs), restic.BlobHandles(nil))

	// check non existent
	test.Equals(t, bs.Has(bh), false)
	_, ok := bs.Get(bh)
	test.Equals(t, false, ok)

	// test insert
	bs.Insert(bh)
	test.Equals(t, bs.Has(bh), true)
	test.Equals(t, bs.Len(), 1)
	test.Equals(t, list(bs), restic.BlobHandles{bh})
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
	test.Equals(t, list(bs), restic.BlobHandles(nil))

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
	test.Equals(t, list(bs), restic.BlobHandles{of, bh})
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
	test.Equals(t, list(bs), restic.BlobHandles{bh})
	test.Equals(t, 0, len(bs.overflow))
}

func TestAssociatedSetWithExtendedIndex(t *testing.T) {
	_, blob := makeFakePackedBlob()

	mi := NewMasterIndex()
	test.OK(t, mi.StorePack(context.TODO(), blob.PackID, []restic.Blob{blob.Blob}, &noopSaver{}))
	test.OK(t, mi.Flush(context.TODO(), &noopSaver{}))

	bs := NewAssociatedSet[uint8](mi)

	// add new blobs to index after building the set
	of, blob2 := makeFakePackedBlob()
	test.OK(t, mi.StorePack(context.TODO(), blob2.PackID, []restic.Blob{blob2.Blob}, &noopSaver{}))
	test.OK(t, mi.Flush(context.TODO(), &noopSaver{}))

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
	test.Equals(t, list(bs), restic.BlobHandles{of})
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
	test.Equals(t, list(bs), restic.BlobHandles(nil))
	test.Equals(t, 0, len(bs.overflow))
}

func TestAssociatedSetIntersectAndSub(t *testing.T) {
	mi := NewMasterIndex()
	saver := &noopSaver{}

	bh1, blob1 := makeFakePackedBlob()
	bh2, blob2 := makeFakePackedBlob()
	bh3, blob3 := makeFakePackedBlob()
	bh4, blob4 := makeFakePackedBlob()

	test.OK(t, mi.StorePack(context.TODO(), blob1.PackID, []restic.Blob{blob1.Blob}, saver))
	test.OK(t, mi.StorePack(context.TODO(), blob2.PackID, []restic.Blob{blob2.Blob}, saver))
	test.OK(t, mi.StorePack(context.TODO(), blob3.PackID, []restic.Blob{blob3.Blob}, saver))
	test.OK(t, mi.StorePack(context.TODO(), blob4.PackID, []restic.Blob{blob4.Blob}, saver))
	test.OK(t, mi.Flush(context.TODO(), saver))

	t.Run("Intersect", func(t *testing.T) {
		bs1, bs2 := NewAssociatedSet[uint8](mi), NewAssociatedSet[uint8](mi)
		test.Equals(t, bs1.Intersect(bs2).Len(), 0)

		bs1, bs2 = NewAssociatedSet[uint8](mi), NewAssociatedSet[uint8](mi)
		bs1.Set(bh1, 10)
		bs2.Set(bh2, 20)
		test.Equals(t, bs1.Intersect(bs2).Len(), 0)

		bs1, bs2 = NewAssociatedSet[uint8](mi), NewAssociatedSet[uint8](mi)
		bs1.Set(bh3, 40)
		bs2.Set(bh3, 50)
		bs2.Set(bh4, 60)
		result := bs1.Intersect(bs2)
		test.Equals(t, result.Len(), 1)
		val, _ := result.Get(bh3)
		test.Equals(t, uint8(40), val)

		bs1, bs2 = NewAssociatedSet[uint8](mi), NewAssociatedSet[uint8](mi)
		bs1.Set(bh3, 40)
		bs1.Set(bh4, 70)
		bs2.Set(bh3, 50)
		bs2.Set(bh4, 60)
		result = bs1.Intersect(bs2)
		test.Equals(t, result.Len(), 2)
		val, _ = result.Get(bh3)
		test.Equals(t, uint8(40), val)
		val, _ = result.Get(bh4)
		test.Equals(t, uint8(70), val)
	})

	t.Run("Sub", func(t *testing.T) {
		bs1, bs2 := NewAssociatedSet[uint8](mi), NewAssociatedSet[uint8](mi)
		test.Equals(t, bs1.Sub(bs2).Len(), 0)

		bs1, bs2 = NewAssociatedSet[uint8](mi), NewAssociatedSet[uint8](mi)
		bs1.Set(bh1, 10)
		bs1.Set(bh2, 20)
		bs2.Set(bh3, 30)
		result := bs1.Sub(bs2)
		test.Equals(t, result.Len(), 2)
		val, _ := result.Get(bh1)
		test.Equals(t, uint8(10), val)
		val, _ = result.Get(bh2)
		test.Equals(t, uint8(20), val)

		bs1, bs2 = NewAssociatedSet[uint8](mi), NewAssociatedSet[uint8](mi)
		bs1.Set(bh1, 10)
		bs1.Set(bh2, 20)
		bs1.Set(bh3, 40)
		bs2.Set(bh2, 50)
		result = bs1.Sub(bs2)
		test.Equals(t, result.Len(), 2)
		test.Assert(t, result.Has(bh1) && result.Has(bh3) && !result.Has(bh2), "only bh1 and bh3 should be in result")

		bs1, bs2 = NewAssociatedSet[uint8](mi), NewAssociatedSet[uint8](mi)
		bs1.Set(bh1, 60)
		bs2.Set(bh1, 70)
		bs2.Set(bh2, 80)
		test.Equals(t, bs1.Sub(bs2).Len(), 0)
	})
}
