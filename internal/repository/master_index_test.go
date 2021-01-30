package repository_test

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/restic/restic/internal/checker"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func TestMasterIndex(t *testing.T) {
	bhInIdx1 := restic.NewRandomBlobHandle()
	bhInIdx2 := restic.NewRandomBlobHandle()
	bhInIdx12 := restic.BlobHandle{ID: restic.NewRandomID(), Type: restic.TreeBlob}

	blob1 := restic.PackedBlob{
		PackID: restic.NewRandomID(),
		Blob: restic.Blob{
			BlobHandle: bhInIdx1,
			Length:     uint(restic.CiphertextLength(10)),
			Offset:     0,
		},
	}

	blob2 := restic.PackedBlob{
		PackID: restic.NewRandomID(),
		Blob: restic.Blob{
			BlobHandle: bhInIdx2,
			Length:     uint(restic.CiphertextLength(100)),
			Offset:     10,
		},
	}

	blob12a := restic.PackedBlob{
		PackID: restic.NewRandomID(),
		Blob: restic.Blob{
			BlobHandle: bhInIdx12,
			Length:     uint(restic.CiphertextLength(123)),
			Offset:     110,
		},
	}

	blob12b := restic.PackedBlob{
		PackID: restic.NewRandomID(),
		Blob: restic.Blob{
			BlobHandle: bhInIdx12,
			Length:     uint(restic.CiphertextLength(123)),
			Offset:     50,
		},
	}

	idx1 := repository.NewIndex()
	idx1.Store(blob1)
	idx1.Store(blob12a)

	idx2 := repository.NewIndex()
	idx2.Store(blob2)
	idx2.Store(blob12b)

	mIdx := repository.NewMasterIndex()
	mIdx.Insert(idx1)
	mIdx.Insert(idx2)

	// test idInIdx1
	found := mIdx.Has(bhInIdx1)
	rtest.Equals(t, true, found)

	blobs := mIdx.Lookup(bhInIdx1)
	rtest.Equals(t, []restic.PackedBlob{blob1}, blobs)

	size, found := mIdx.LookupSize(bhInIdx1)
	rtest.Equals(t, true, found)
	rtest.Equals(t, uint(10), size)

	// test idInIdx2
	found = mIdx.Has(bhInIdx2)
	rtest.Equals(t, true, found)

	blobs = mIdx.Lookup(bhInIdx2)
	rtest.Equals(t, []restic.PackedBlob{blob2}, blobs)

	size, found = mIdx.LookupSize(bhInIdx2)
	rtest.Equals(t, true, found)
	rtest.Equals(t, uint(100), size)

	// test idInIdx12
	found = mIdx.Has(bhInIdx12)
	rtest.Equals(t, true, found)

	blobs = mIdx.Lookup(bhInIdx12)
	rtest.Equals(t, 2, len(blobs))

	// test Lookup result for blob12a
	found = false
	if blobs[0] == blob12a || blobs[1] == blob12a {
		found = true
	}
	rtest.Assert(t, found, "blob12a not found in result")

	// test Lookup result for blob12b
	found = false
	if blobs[0] == blob12b || blobs[1] == blob12b {
		found = true
	}
	rtest.Assert(t, found, "blob12a not found in result")

	size, found = mIdx.LookupSize(bhInIdx12)
	rtest.Equals(t, true, found)
	rtest.Equals(t, uint(123), size)

	// test not in index
	found = mIdx.Has(restic.BlobHandle{ID: restic.NewRandomID(), Type: restic.TreeBlob})
	rtest.Assert(t, !found, "Expected no blobs when fetching with a random id")
	blobs = mIdx.Lookup(restic.NewRandomBlobHandle())
	rtest.Assert(t, blobs == nil, "Expected no blobs when fetching with a random id")
	_, found = mIdx.LookupSize(restic.NewRandomBlobHandle())
	rtest.Assert(t, !found, "Expected no blobs when fetching with a random id")

	// Test Count
	num := mIdx.Count(restic.DataBlob)
	rtest.Equals(t, uint(2), num)
	num = mIdx.Count(restic.TreeBlob)
	rtest.Equals(t, uint(2), num)
}

func TestMasterMergeFinalIndexes(t *testing.T) {
	bhInIdx1 := restic.NewRandomBlobHandle()
	bhInIdx2 := restic.NewRandomBlobHandle()

	blob1 := restic.PackedBlob{
		PackID: restic.NewRandomID(),
		Blob: restic.Blob{
			BlobHandle: bhInIdx1,
			Length:     10,
			Offset:     0,
		},
	}

	blob2 := restic.PackedBlob{
		PackID: restic.NewRandomID(),
		Blob: restic.Blob{
			BlobHandle: bhInIdx2,
			Length:     100,
			Offset:     10,
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

	err := mIdx.MergeFinalIndexes()
	if err != nil {
		t.Fatal(err)
	}

	allIndexes := mIdx.All()
	rtest.Equals(t, 1, len(allIndexes))

	blobCount := 0
	for range mIdx.Each(context.TODO()) {
		blobCount++
	}
	rtest.Equals(t, 2, blobCount)

	blobs := mIdx.Lookup(bhInIdx1)
	rtest.Equals(t, []restic.PackedBlob{blob1}, blobs)

	blobs = mIdx.Lookup(bhInIdx2)
	rtest.Equals(t, []restic.PackedBlob{blob2}, blobs)

	blobs = mIdx.Lookup(restic.NewRandomBlobHandle())
	rtest.Assert(t, blobs == nil, "Expected no blobs when fetching with a random id")

	// merge another index containing identical blobs
	idx3 := repository.NewIndex()
	idx3.Store(blob1)
	idx3.Store(blob2)

	mIdx.Insert(idx3)
	finalIndexes = mIdx.FinalizeNotFinalIndexes()
	rtest.Equals(t, []*repository.Index{idx3}, finalIndexes)

	err = mIdx.MergeFinalIndexes()
	if err != nil {
		t.Fatal(err)
	}

	allIndexes = mIdx.All()
	rtest.Equals(t, 1, len(allIndexes))

	// Index should have same entries as before!
	blobs = mIdx.Lookup(bhInIdx1)
	rtest.Equals(t, []restic.PackedBlob{blob1}, blobs)

	blobs = mIdx.Lookup(bhInIdx2)
	rtest.Equals(t, []restic.PackedBlob{blob2}, blobs)

	blobCount = 0
	for range mIdx.Each(context.TODO()) {
		blobCount++
	}
	rtest.Equals(t, 2, blobCount)
}

func createRandomMasterIndex(t testing.TB, rng *rand.Rand, num, size int) (*repository.MasterIndex, restic.BlobHandle) {
	mIdx := repository.NewMasterIndex()
	for i := 0; i < num-1; i++ {
		idx, _ := createRandomIndex(rng, size)
		mIdx.Insert(idx)
	}
	idx1, lookupBh := createRandomIndex(rng, size)
	mIdx.Insert(idx1)

	mIdx.FinalizeNotFinalIndexes()
	err := mIdx.MergeFinalIndexes()
	if err != nil {
		t.Fatal(err)
	}

	return mIdx, lookupBh
}

func BenchmarkMasterIndexAlloc(b *testing.B) {
	rng := rand.New(rand.NewSource(0))
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		createRandomMasterIndex(b, rng, 10000, 5)
	}
}

func BenchmarkMasterIndexLookupSingleIndex(b *testing.B) {
	mIdx, lookupBh := createRandomMasterIndex(b, rand.New(rand.NewSource(0)), 1, 200000)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		mIdx.Lookup(lookupBh)
	}
}

func BenchmarkMasterIndexLookupMultipleIndex(b *testing.B) {
	mIdx, lookupBh := createRandomMasterIndex(b, rand.New(rand.NewSource(0)), 100, 10000)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		mIdx.Lookup(lookupBh)
	}
}

func BenchmarkMasterIndexLookupSingleIndexUnknown(b *testing.B) {

	lookupBh := restic.NewRandomBlobHandle()
	mIdx, _ := createRandomMasterIndex(b, rand.New(rand.NewSource(0)), 1, 200000)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		mIdx.Lookup(lookupBh)
	}
}

func BenchmarkMasterIndexLookupMultipleIndexUnknown(b *testing.B) {
	lookupBh := restic.NewRandomBlobHandle()
	mIdx, _ := createRandomMasterIndex(b, rand.New(rand.NewSource(0)), 100, 10000)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		mIdx.Lookup(lookupBh)
	}
}

func BenchmarkMasterIndexLookupParallel(b *testing.B) {
	mIdx := repository.NewMasterIndex()

	for _, numindices := range []int{25, 50, 100} {
		var lookupBh restic.BlobHandle

		b.StopTimer()
		rng := rand.New(rand.NewSource(0))
		mIdx, lookupBh = createRandomMasterIndex(b, rng, numindices, 10000)
		b.StartTimer()

		name := fmt.Sprintf("known,indices=%d", numindices)
		b.Run(name, func(b *testing.B) {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					mIdx.Lookup(lookupBh)
				}
			})
		})

		lookupBh = restic.NewRandomBlobHandle()
		name = fmt.Sprintf("unknown,indices=%d", numindices)
		b.Run(name, func(b *testing.B) {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					mIdx.Lookup(lookupBh)
				}
			})
		})
	}
}

func BenchmarkMasterIndexLookupBlobSize(b *testing.B) {
	rng := rand.New(rand.NewSource(0))
	mIdx, lookupBh := createRandomMasterIndex(b, rand.New(rng), 5, 200000)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		mIdx.LookupSize(lookupBh)
	}
}

var (
	snapshotTime = time.Unix(1470492820, 207401672)
	depth        = 3
)

func createFilledRepo(t testing.TB, snapshots int, dup float32) (restic.Repository, func()) {
	repo, cleanup := repository.TestRepository(t)

	for i := 0; i < 3; i++ {
		restic.TestCreateSnapshot(t, repo, snapshotTime.Add(time.Duration(i)*time.Second), depth, dup)
	}

	return repo, cleanup
}

func TestIndexSave(t *testing.T) {
	repo, cleanup := createFilledRepo(t, 3, 0)
	defer cleanup()

	err := repo.LoadIndex(context.TODO())
	if err != nil {
		t.Fatal(err)
	}

	obsoletes, err := repo.Index().(*repository.MasterIndex).Save(context.TODO(), repo, nil, nil, nil)
	if err != nil {
		t.Fatalf("unable to save new index: %v", err)
	}

	for id := range obsoletes {
		t.Logf("remove index %v", id.Str())
		h := restic.Handle{Type: restic.IndexFile, Name: id.String()}
		err = repo.Backend().Remove(context.TODO(), h)
		if err != nil {
			t.Errorf("error removing index %v: %v", id, err)
		}
	}

	checker := checker.New(repo, false)
	hints, errs := checker.LoadIndex(context.TODO())
	for _, h := range hints {
		t.Logf("hint: %v\n", h)
	}

	for _, err := range errs {
		t.Errorf("checker found error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	errCh := make(chan error)
	go checker.Structure(ctx, nil, errCh)
	i := 0
	for err := range errCh {
		t.Errorf("checker returned error: %v", err)
		i++
		if i == 10 {
			t.Errorf("more than 10 errors returned, skipping the rest")
			cancel()
			break
		}
	}
}
