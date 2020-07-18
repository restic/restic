package repository_test

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func TestMasterIndexLookup(t *testing.T) {
	idInIdx1 := restic.NewRandomID()
	idInIdx2 := restic.NewRandomID()

	blob1 := restic.PackedBlob{
		PackID: restic.NewRandomID(),
		Blob: restic.Blob{
			Type:   restic.DataBlob,
			ID:     idInIdx1,
			Length: 10,
			Offset: 0,
		},
	}

	blob2 := restic.PackedBlob{
		PackID: restic.NewRandomID(),
		Blob: restic.Blob{
			Type:   restic.DataBlob,
			ID:     idInIdx2,
			Length: 100,
			Offset: 10,
		},
	}

	idx1 := repository.NewIndex()
	idx1.Store(blob1)

	idx2 := repository.NewIndex()
	idx2.Store(blob2)

	mIdx := repository.NewMasterIndex()
	mIdx.Insert(idx1)
	mIdx.Insert(idx2)

	blobs, found := mIdx.Lookup(idInIdx1, restic.DataBlob)
	rtest.Assert(t, found, "Expected to find blob id %v from index 1", idInIdx1)
	rtest.Equals(t, []restic.PackedBlob{blob1}, blobs)

	blobs, found = mIdx.Lookup(idInIdx2, restic.DataBlob)
	rtest.Assert(t, found, "Expected to find blob id %v from index 2", idInIdx2)
	rtest.Equals(t, []restic.PackedBlob{blob2}, blobs)

	blobs, found = mIdx.Lookup(restic.NewRandomID(), restic.DataBlob)
	rtest.Assert(t, !found, "Expected to not find a blob when fetching with a random id")
	rtest.Assert(t, blobs == nil, "Expected no blobs when fetching with a random id")
}

func TestMasterMergeFinalIndexes(t *testing.T) {
	idInIdx1 := restic.NewRandomID()
	idInIdx2 := restic.NewRandomID()

	blob1 := restic.PackedBlob{
		PackID: restic.NewRandomID(),
		Blob: restic.Blob{
			Type:   restic.DataBlob,
			ID:     idInIdx1,
			Length: 10,
			Offset: 0,
		},
	}

	blob2 := restic.PackedBlob{
		PackID: restic.NewRandomID(),
		Blob: restic.Blob{
			Type:   restic.DataBlob,
			ID:     idInIdx2,
			Length: 100,
			Offset: 10,
		},
	}

	idx1 := repository.NewIndex()
	idx1.Store(blob1)

	idx2 := repository.NewIndex()
	idx2.Store(blob2)

	mIdx := repository.NewMasterIndex()
	mIdx.Insert(idx1)
	mIdx.Insert(idx2)

	finalIndexes := mIdx.FinalizeNotFinalIndexes()
	rtest.Equals(t, []*repository.Index{idx1, idx2}, finalIndexes)

	mIdx.MergeFinalIndexes()

	blobs, found := mIdx.Lookup(idInIdx1, restic.DataBlob)
	rtest.Assert(t, found, "Expected to find blob id %v from index 1", idInIdx1)
	rtest.Equals(t, []restic.PackedBlob{blob1}, blobs)

	blobs, found = mIdx.Lookup(idInIdx2, restic.DataBlob)
	rtest.Assert(t, found, "Expected to find blob id %v from index 2", idInIdx2)
	rtest.Equals(t, []restic.PackedBlob{blob2}, blobs)

	blobs, found = mIdx.Lookup(restic.NewRandomID(), restic.DataBlob)
	rtest.Assert(t, !found, "Expected to not find a blob when fetching with a random id")
	rtest.Assert(t, blobs == nil, "Expected no blobs when fetching with a random id")
}

func createRandomMasterIndex(rng *rand.Rand, num, size int) (*repository.MasterIndex, restic.ID) {
	mIdx := repository.NewMasterIndex()
	for i := 0; i < num-1; i++ {
		idx, _ := createRandomIndex(rng, size)
		mIdx.Insert(idx)
	}
	idx1, lookupID := createRandomIndex(rng, size)
	mIdx.Insert(idx1)

	mIdx.FinalizeNotFinalIndexes()
	mIdx.MergeFinalIndexes()

	return mIdx, lookupID
}

func BenchmarkMasterIndexLookupSingleIndex(b *testing.B) {
	mIdx, lookupID := createRandomMasterIndex(rand.New(rand.NewSource(0)), 1, 200000)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		mIdx.Lookup(lookupID, restic.DataBlob)
	}
}

func BenchmarkMasterIndexLookupMultipleIndex(b *testing.B) {
	mIdx, lookupID := createRandomMasterIndex(rand.New(rand.NewSource(0)), 100, 10000)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		mIdx.Lookup(lookupID, restic.DataBlob)
	}
}

func BenchmarkMasterIndexLookupSingleIndexUnknown(b *testing.B) {

	lookupID := restic.NewRandomID()
	mIdx, _ := createRandomMasterIndex(rand.New(rand.NewSource(0)), 1, 200000)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		mIdx.Lookup(lookupID, restic.DataBlob)
	}
}

func BenchmarkMasterIndexLookupMultipleIndexUnknown(b *testing.B) {
	lookupID := restic.NewRandomID()
	mIdx, _ := createRandomMasterIndex(rand.New(rand.NewSource(0)), 100, 10000)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		mIdx.Lookup(lookupID, restic.DataBlob)
	}
}

func BenchmarkMasterIndexLookupParallel(b *testing.B) {
	mIdx := repository.NewMasterIndex()

	for _, numindices := range []int{25, 50, 100} {
		var lookupID restic.ID

		b.StopTimer()
		rng := rand.New(rand.NewSource(0))
		mIdx, lookupID = createRandomMasterIndex(rng, numindices, 10000)
		b.StartTimer()

		name := fmt.Sprintf("known,indices=%d", numindices)
		b.Run(name, func(b *testing.B) {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					mIdx.Lookup(lookupID, restic.DataBlob)
				}
			})
		})

		lookupID = restic.NewRandomID()
		name = fmt.Sprintf("unknown,indices=%d", numindices)
		b.Run(name, func(b *testing.B) {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					mIdx.Lookup(lookupID, restic.DataBlob)
				}
			})
		})

	}
}
