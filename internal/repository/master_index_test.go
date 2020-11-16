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
	idInIdx1 := restic.NewRandomID()
	idInIdx2 := restic.NewRandomID()
	idInIdx12 := restic.NewRandomID()

	blob1 := restic.PackedBlob{
		PackID: restic.NewRandomID(),
		Blob: restic.Blob{
			Type:   restic.DataBlob,
			ID:     idInIdx1,
			Length: uint(restic.CiphertextLength(10)),
			Offset: 0,
		},
	}

	blob2 := restic.PackedBlob{
		PackID: restic.NewRandomID(),
		Blob: restic.Blob{
			Type:   restic.DataBlob,
			ID:     idInIdx2,
			Length: uint(restic.CiphertextLength(100)),
			Offset: 10,
		},
	}

	blob12a := restic.PackedBlob{
		PackID: restic.NewRandomID(),
		Blob: restic.Blob{
			Type:   restic.TreeBlob,
			ID:     idInIdx12,
			Length: uint(restic.CiphertextLength(123)),
			Offset: 110,
		},
	}

	blob12b := restic.PackedBlob{
		PackID: restic.NewRandomID(),
		Blob: restic.Blob{
			Type:   restic.TreeBlob,
			ID:     idInIdx12,
			Length: uint(restic.CiphertextLength(123)),
			Offset: 50,
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
	found := mIdx.Has(idInIdx1, restic.DataBlob)
	rtest.Equals(t, true, found)

	blobs := mIdx.Lookup(idInIdx1, restic.DataBlob)
	rtest.Equals(t, []restic.PackedBlob{blob1}, blobs)

	size, found := mIdx.LookupSize(idInIdx1, restic.DataBlob)
	rtest.Equals(t, true, found)
	rtest.Equals(t, uint(10), size)

	// test idInIdx2
	found = mIdx.Has(idInIdx2, restic.DataBlob)
	rtest.Equals(t, true, found)

	blobs = mIdx.Lookup(idInIdx2, restic.DataBlob)
	rtest.Equals(t, []restic.PackedBlob{blob2}, blobs)

	size, found = mIdx.LookupSize(idInIdx2, restic.DataBlob)
	rtest.Equals(t, true, found)
	rtest.Equals(t, uint(100), size)

	// test idInIdx12
	found = mIdx.Has(idInIdx12, restic.TreeBlob)
	rtest.Equals(t, true, found)

	blobs = mIdx.Lookup(idInIdx12, restic.TreeBlob)
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

	size, found = mIdx.LookupSize(idInIdx12, restic.TreeBlob)
	rtest.Equals(t, true, found)
	rtest.Equals(t, uint(123), size)

	// test not in index
	found = mIdx.Has(restic.NewRandomID(), restic.TreeBlob)
	rtest.Assert(t, !found, "Expected no blobs when fetching with a random id")
	blobs = mIdx.Lookup(restic.NewRandomID(), restic.DataBlob)
	rtest.Assert(t, blobs == nil, "Expected no blobs when fetching with a random id")
	_, found = mIdx.LookupSize(restic.NewRandomID(), restic.DataBlob)
	rtest.Assert(t, !found, "Expected no blobs when fetching with a random id")

	// Test Count
	num := mIdx.Count(restic.DataBlob)
	rtest.Equals(t, uint(2), num)
	num = mIdx.Count(restic.TreeBlob)
	rtest.Equals(t, uint(2), num)
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
	allIndexes := mIdx.All()
	rtest.Equals(t, 1, len(allIndexes))

	blobCount := 0
	for range mIdx.Each(context.TODO()) {
		blobCount++
	}
	rtest.Equals(t, 2, blobCount)

	blobs := mIdx.Lookup(idInIdx1, restic.DataBlob)
	rtest.Equals(t, []restic.PackedBlob{blob1}, blobs)

	blobs = mIdx.Lookup(idInIdx2, restic.DataBlob)
	rtest.Equals(t, []restic.PackedBlob{blob2}, blobs)

	blobs = mIdx.Lookup(restic.NewRandomID(), restic.DataBlob)
	rtest.Assert(t, blobs == nil, "Expected no blobs when fetching with a random id")

	// merge another index containing identical blobs
	idx3 := repository.NewIndex()
	idx3.Store(blob1)
	idx3.Store(blob2)

	mIdx.Insert(idx3)
	finalIndexes = mIdx.FinalizeNotFinalIndexes()
	rtest.Equals(t, []*repository.Index{idx3}, finalIndexes)

	mIdx.MergeFinalIndexes()
	allIndexes = mIdx.All()
	rtest.Equals(t, 1, len(allIndexes))

	// Index should have same entries as before!
	blobs = mIdx.Lookup(idInIdx1, restic.DataBlob)
	rtest.Equals(t, []restic.PackedBlob{blob1}, blobs)

	blobs = mIdx.Lookup(idInIdx2, restic.DataBlob)
	rtest.Equals(t, []restic.PackedBlob{blob2}, blobs)

	blobCount = 0
	for range mIdx.Each(context.TODO()) {
		blobCount++
	}
	rtest.Equals(t, 2, blobCount)
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

func BenchmarkMasterIndexAlloc(b *testing.B) {
	rng := rand.New(rand.NewSource(0))
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		createRandomMasterIndex(rng, 10000, 5)
	}
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

func BenchmarkMasterIndexLookupBlobSize(b *testing.B) {
	rng := rand.New(rand.NewSource(0))
	mIdx, lookupID := createRandomMasterIndex(rand.New(rng), 5, 200000)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		mIdx.LookupSize(lookupID, restic.DataBlob)
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

	repo.LoadIndex(context.TODO())

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
	go checker.Structure(ctx, errCh)
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
