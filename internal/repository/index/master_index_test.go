package index_test

import (
	"context"
	"fmt"
	"math/rand"
	"runtime"
	"testing"
	"time"

	"github.com/restic/restic/internal/checker"
	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/repository/index"
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
			Length:     uint(crypto.CiphertextLength(10)),
			Offset:     0,
		},
	}

	blob2 := restic.PackedBlob{
		PackID: restic.NewRandomID(),
		Blob: restic.Blob{
			BlobHandle:         bhInIdx2,
			Length:             uint(crypto.CiphertextLength(100)),
			Offset:             10,
			UncompressedLength: 200,
		},
	}

	blob12a := restic.PackedBlob{
		PackID: restic.NewRandomID(),
		Blob: restic.Blob{
			BlobHandle:         bhInIdx12,
			Length:             uint(crypto.CiphertextLength(123)),
			Offset:             110,
			UncompressedLength: 80,
		},
	}

	blob12b := restic.PackedBlob{
		PackID: restic.NewRandomID(),
		Blob: restic.Blob{
			BlobHandle:         bhInIdx12,
			Length:             uint(crypto.CiphertextLength(123)),
			Offset:             50,
			UncompressedLength: 80,
		},
	}

	idx1 := index.NewIndex()
	idx1.StorePack(blob1.PackID, []restic.Blob{blob1.Blob})
	idx1.StorePack(blob12a.PackID, []restic.Blob{blob12a.Blob})

	idx2 := index.NewIndex()
	idx2.StorePack(blob2.PackID, []restic.Blob{blob2.Blob})
	idx2.StorePack(blob12b.PackID, []restic.Blob{blob12b.Blob})

	mIdx := index.NewMasterIndex()
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
	rtest.Equals(t, uint(200), size)

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
	rtest.Equals(t, uint(80), size)

	// test not in index
	found = mIdx.Has(restic.BlobHandle{ID: restic.NewRandomID(), Type: restic.TreeBlob})
	rtest.Assert(t, !found, "Expected no blobs when fetching with a random id")
	blobs = mIdx.Lookup(restic.NewRandomBlobHandle())
	rtest.Assert(t, blobs == nil, "Expected no blobs when fetching with a random id")
	_, found = mIdx.LookupSize(restic.NewRandomBlobHandle())
	rtest.Assert(t, !found, "Expected no blobs when fetching with a random id")
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
			BlobHandle:         bhInIdx2,
			Length:             100,
			Offset:             10,
			UncompressedLength: 200,
		},
	}

	idx1 := index.NewIndex()
	idx1.StorePack(blob1.PackID, []restic.Blob{blob1.Blob})

	idx2 := index.NewIndex()
	idx2.StorePack(blob2.PackID, []restic.Blob{blob2.Blob})

	mIdx := index.NewMasterIndex()
	mIdx.Insert(idx1)
	mIdx.Insert(idx2)

	rtest.Equals(t, restic.NewIDSet(), mIdx.IDs())

	finalIndexes, idxCount, ids := index.TestMergeIndex(t, mIdx)
	rtest.Equals(t, []*index.Index{idx1, idx2}, finalIndexes)
	rtest.Equals(t, 1, idxCount)
	rtest.Equals(t, ids, mIdx.IDs())

	blobCount := 0
	rtest.OK(t, mIdx.Each(context.TODO(), func(pb restic.PackedBlob) {
		blobCount++
	}))
	rtest.Equals(t, 2, blobCount)

	blobs := mIdx.Lookup(bhInIdx1)
	rtest.Equals(t, []restic.PackedBlob{blob1}, blobs)

	blobs = mIdx.Lookup(bhInIdx2)
	rtest.Equals(t, []restic.PackedBlob{blob2}, blobs)

	blobs = mIdx.Lookup(restic.NewRandomBlobHandle())
	rtest.Assert(t, blobs == nil, "Expected no blobs when fetching with a random id")

	// merge another index containing identical blobs
	idx3 := index.NewIndex()
	idx3.StorePack(blob1.PackID, []restic.Blob{blob1.Blob})
	idx3.StorePack(blob2.PackID, []restic.Blob{blob2.Blob})

	mIdx.Insert(idx3)
	finalIndexes, idxCount, newIDs := index.TestMergeIndex(t, mIdx)
	rtest.Equals(t, []*index.Index{idx3}, finalIndexes)
	rtest.Equals(t, 1, idxCount)
	ids.Merge(newIDs)
	rtest.Equals(t, ids, mIdx.IDs())

	// Index should have same entries as before!
	blobs = mIdx.Lookup(bhInIdx1)
	rtest.Equals(t, []restic.PackedBlob{blob1}, blobs)

	blobs = mIdx.Lookup(bhInIdx2)
	rtest.Equals(t, []restic.PackedBlob{blob2}, blobs)

	blobCount = 0
	rtest.OK(t, mIdx.Each(context.TODO(), func(pb restic.PackedBlob) {
		blobCount++
	}))
	rtest.Equals(t, 2, blobCount)
}

func createRandomMasterIndex(t testing.TB, rng *rand.Rand, num, size int) (*index.MasterIndex, restic.BlobHandle) {
	mIdx := index.NewMasterIndex()
	for i := 0; i < num-1; i++ {
		idx, _ := createRandomIndex(rng, size)
		mIdx.Insert(idx)
	}
	idx1, lookupBh := createRandomIndex(rng, size)
	mIdx.Insert(idx1)

	index.TestMergeIndex(t, mIdx)

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
	for _, numindices := range []int{25, 50, 100} {
		var lookupBh restic.BlobHandle

		b.StopTimer()
		rng := rand.New(rand.NewSource(0))
		mIdx, lookupBh := createRandomMasterIndex(b, rng, numindices, 10000)
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

func BenchmarkMasterIndexEach(b *testing.B) {
	rng := rand.New(rand.NewSource(0))
	mIdx, _ := createRandomMasterIndex(b, rand.New(rng), 5, 200000)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		entries := 0
		rtest.OK(b, mIdx.Each(context.TODO(), func(pb restic.PackedBlob) {
			entries++
		}))
	}
}

func BenchmarkMasterIndexGC(b *testing.B) {
	mIdx, _ := createRandomMasterIndex(b, rand.New(rand.NewSource(0)), 100, 10000)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		runtime.GC()
	}
	runtime.KeepAlive(mIdx)
}

var (
	snapshotTime = time.Unix(1470492820, 207401672)
	depth        = 3
)

func createFilledRepo(t testing.TB, snapshots int, version uint) restic.Repository {
	repo, _ := repository.TestRepositoryWithVersion(t, version)

	for i := 0; i < snapshots; i++ {
		restic.TestCreateSnapshot(t, repo, snapshotTime.Add(time.Duration(i)*time.Second), depth)
	}
	return repo
}

func TestIndexSave(t *testing.T) {
	repository.TestAllVersions(t, testIndexSave)
}

func testIndexSave(t *testing.T, version uint) {
	for _, test := range []struct {
		name  string
		saver func(idx *index.MasterIndex, repo restic.Repository) error
	}{
		{"rewrite no-op", func(idx *index.MasterIndex, repo restic.Repository) error {
			return idx.Rewrite(context.TODO(), repo, nil, nil, nil, index.MasterIndexRewriteOpts{})
		}},
		{"rewrite skip-all", func(idx *index.MasterIndex, repo restic.Repository) error {
			return idx.Rewrite(context.TODO(), repo, nil, restic.NewIDSet(), nil, index.MasterIndexRewriteOpts{})
		}},
		{"SaveFallback", func(idx *index.MasterIndex, repo restic.Repository) error {
			err := restic.ParallelRemove(context.TODO(), repo, idx.IDs(), restic.IndexFile, nil, nil)
			if err != nil {
				return nil
			}
			return idx.SaveFallback(context.TODO(), repo, restic.NewIDSet(), nil)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := createFilledRepo(t, 3, version)

			idx := index.NewMasterIndex()
			rtest.OK(t, idx.Load(context.TODO(), repo, nil, nil))
			blobs := make(map[restic.PackedBlob]struct{})
			rtest.OK(t, idx.Each(context.TODO(), func(pb restic.PackedBlob) {
				blobs[pb] = struct{}{}
			}))

			rtest.OK(t, test.saver(idx, repo))
			idx = index.NewMasterIndex()
			rtest.OK(t, idx.Load(context.TODO(), repo, nil, nil))

			rtest.OK(t, idx.Each(context.TODO(), func(pb restic.PackedBlob) {
				if _, ok := blobs[pb]; ok {
					delete(blobs, pb)
				} else {
					t.Fatalf("unexpected blobs %v", pb)
				}
			}))
			rtest.Equals(t, 0, len(blobs), "saved index is missing blobs")

			checker.TestCheckRepo(t, repo, false)
		})
	}
}

func TestIndexSavePartial(t *testing.T) {
	repository.TestAllVersions(t, testIndexSavePartial)
}

func testIndexSavePartial(t *testing.T, version uint) {
	repo := createFilledRepo(t, 3, version)

	// capture blob list before adding fourth snapshot
	idx := index.NewMasterIndex()
	rtest.OK(t, idx.Load(context.TODO(), repo, nil, nil))
	blobs := make(map[restic.PackedBlob]struct{})
	rtest.OK(t, idx.Each(context.TODO(), func(pb restic.PackedBlob) {
		blobs[pb] = struct{}{}
	}))

	// add+remove new snapshot and track its pack files
	packsBefore := listPacks(t, repo)
	sn := restic.TestCreateSnapshot(t, repo, snapshotTime.Add(time.Duration(4)*time.Second), depth)
	rtest.OK(t, repo.RemoveUnpacked(context.TODO(), restic.SnapshotFile, *sn.ID()))
	packsAfter := listPacks(t, repo)
	newPacks := packsAfter.Sub(packsBefore)

	// rewrite index and remove pack files of new snapshot
	idx = index.NewMasterIndex()
	rtest.OK(t, idx.Load(context.TODO(), repo, nil, nil))
	rtest.OK(t, idx.Rewrite(context.TODO(), repo, newPacks, nil, nil, index.MasterIndexRewriteOpts{}))

	// check blobs
	idx = index.NewMasterIndex()
	rtest.OK(t, idx.Load(context.TODO(), repo, nil, nil))
	rtest.OK(t, idx.Each(context.TODO(), func(pb restic.PackedBlob) {
		if _, ok := blobs[pb]; ok {
			delete(blobs, pb)
		} else {
			t.Fatalf("unexpected blobs %v", pb)
		}
	}))
	rtest.Equals(t, 0, len(blobs), "saved index is missing blobs")

	// remove pack files to make check happy
	rtest.OK(t, restic.ParallelRemove(context.TODO(), repo, newPacks, restic.PackFile, nil, nil))

	checker.TestCheckRepo(t, repo, false)
}

func listPacks(t testing.TB, repo restic.Lister) restic.IDSet {
	s := restic.NewIDSet()
	rtest.OK(t, repo.List(context.TODO(), restic.PackFile, func(id restic.ID, _ int64) error {
		s.Insert(id)
		return nil
	}))
	return s
}
