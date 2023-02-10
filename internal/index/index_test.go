package index_test

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"sync"
	"testing"

	"github.com/restic/restic/internal/index"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func TestIndexSerialize(t *testing.T) {
	tests := []restic.PackedBlob{}

	idx := index.NewIndex()

	// create 50 packs with 20 blobs each
	for i := 0; i < 50; i++ {
		packID := restic.NewRandomID()
		var blobs []restic.Blob

		pos := uint(0)
		for j := 0; j < 20; j++ {
			length := uint(i*100 + j)
			uncompressedLength := uint(0)
			if i >= 25 {
				// test a mix of compressed and uncompressed packs
				uncompressedLength = 2 * length
			}
			pb := restic.PackedBlob{
				Blob: restic.Blob{
					BlobHandle:         restic.NewRandomBlobHandle(),
					Offset:             pos,
					Length:             length,
					UncompressedLength: uncompressedLength,
				},
				PackID: packID,
			}
			blobs = append(blobs, pb.Blob)
			tests = append(tests, pb)
			pos += length
		}
		idx.StorePack(packID, blobs)
	}

	wr := bytes.NewBuffer(nil)
	err := idx.Encode(wr)
	rtest.OK(t, err)

	idx2ID := restic.NewRandomID()
	idx2, oldFormat, err := index.DecodeIndex(wr.Bytes(), idx2ID)
	rtest.OK(t, err)
	rtest.Assert(t, idx2 != nil,
		"nil returned for decoded index")
	rtest.Assert(t, !oldFormat, "new index format recognized as old format")
	indexID, err := idx2.IDs()
	rtest.OK(t, err)
	rtest.Equals(t, indexID, restic.IDs{idx2ID})

	wr2 := bytes.NewBuffer(nil)
	err = idx2.Encode(wr2)
	rtest.OK(t, err)

	for _, testBlob := range tests {
		list := idx.Lookup(testBlob.BlobHandle, nil)
		if len(list) != 1 {
			t.Errorf("expected one result for blob %v, got %v: %v", testBlob.ID.Str(), len(list), list)
		}
		result := list[0]

		rtest.Equals(t, testBlob, result)

		list2 := idx2.Lookup(testBlob.BlobHandle, nil)
		if len(list2) != 1 {
			t.Errorf("expected one result for blob %v, got %v: %v", testBlob.ID.Str(), len(list2), list2)
		}
		result2 := list2[0]

		rtest.Equals(t, testBlob, result2)
	}

	// add more blobs to idx
	newtests := []restic.PackedBlob{}
	for i := 0; i < 10; i++ {
		packID := restic.NewRandomID()
		var blobs []restic.Blob

		pos := uint(0)
		for j := 0; j < 10; j++ {
			length := uint(i*100 + j)
			pb := restic.PackedBlob{
				Blob: restic.Blob{
					BlobHandle: restic.NewRandomBlobHandle(),
					Offset:     pos,
					Length:     length,
				},
				PackID: packID,
			}
			blobs = append(blobs, pb.Blob)
			newtests = append(newtests, pb)
			pos += length
		}
		idx.StorePack(packID, blobs)
	}

	// finalize; serialize idx, unserialize to idx3
	idx.Finalize()
	wr3 := bytes.NewBuffer(nil)
	err = idx.Encode(wr3)
	rtest.OK(t, err)

	rtest.Assert(t, idx.Final(),
		"index not final after encoding")

	id := restic.NewRandomID()
	rtest.OK(t, idx.SetID(id))
	ids, err := idx.IDs()
	rtest.OK(t, err)
	rtest.Equals(t, restic.IDs{id}, ids)

	idx3, oldFormat, err := index.DecodeIndex(wr3.Bytes(), id)
	rtest.OK(t, err)
	rtest.Assert(t, idx3 != nil,
		"nil returned for decoded index")
	rtest.Assert(t, idx3.Final(),
		"decoded index is not final")
	rtest.Assert(t, !oldFormat, "new index format recognized as old format")

	// all new blobs must be in the index
	for _, testBlob := range newtests {
		list := idx3.Lookup(testBlob.BlobHandle, nil)
		if len(list) != 1 {
			t.Errorf("expected one result for blob %v, got %v: %v", testBlob.ID.Str(), len(list), list)
		}

		blob := list[0]

		rtest.Equals(t, testBlob, blob)
	}
}

func TestIndexSize(t *testing.T) {
	idx := index.NewIndex()

	packs := 200
	blobCount := 100
	for i := 0; i < packs; i++ {
		packID := restic.NewRandomID()
		var blobs []restic.Blob

		pos := uint(0)
		for j := 0; j < blobCount; j++ {
			length := uint(i*100 + j)
			blobs = append(blobs, restic.Blob{
				BlobHandle: restic.NewRandomBlobHandle(),
				Offset:     pos,
				Length:     length,
			})

			pos += length
		}
		idx.StorePack(packID, blobs)
	}

	wr := bytes.NewBuffer(nil)

	err := idx.Encode(wr)
	rtest.OK(t, err)

	t.Logf("Index file size for %d blobs in %d packs is %d", blobCount*packs, packs, wr.Len())
}

// example index serialization from doc/Design.rst
var docExampleV1 = []byte(`
{
  "supersedes": [
	"ed54ae36197f4745ebc4b54d10e0f623eaaaedd03013eb7ae90df881b7781452"
  ],
  "packs": [
	{
	  "id": "73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c",
	  "blobs": [
		{
		  "id": "3ec79977ef0cf5de7b08cd12b874cd0f62bbaf7f07f3497a5b1bbcc8cb39b1ce",
		  "type": "data",
		  "offset": 0,
		  "length": 38
		},{
		  "id": "9ccb846e60d90d4eb915848add7aa7ea1e4bbabfc60e573db9f7bfb2789afbae",
		  "type": "tree",
		  "offset": 38,
		  "length": 112
		},
		{
		  "id": "d3dc577b4ffd38cc4b32122cabf8655a0223ed22edfd93b353dc0c3f2b0fdf66",
		  "type": "data",
		  "offset": 150,
		  "length": 123
		}
	  ]
	}
  ]
}
`)

var docExampleV2 = []byte(`
{
	"supersedes": [
	  "ed54ae36197f4745ebc4b54d10e0f623eaaaedd03013eb7ae90df881b7781452"
	],
	"packs": [
	  {
		"id": "73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c",
		"blobs": [
		  {
			"id": "3ec79977ef0cf5de7b08cd12b874cd0f62bbaf7f07f3497a5b1bbcc8cb39b1ce",
			"type": "data",
			"offset": 0,
			"length": 38
		  },
		  {
			"id": "9ccb846e60d90d4eb915848add7aa7ea1e4bbabfc60e573db9f7bfb2789afbae",
			"type": "tree",
			"offset": 38,
			"length": 112,
			"uncompressed_length": 511
		  },
		  {
			"id": "d3dc577b4ffd38cc4b32122cabf8655a0223ed22edfd93b353dc0c3f2b0fdf66",
			"type": "data",
			"offset": 150,
			"length": 123,
			"uncompressed_length": 234
		  }
		]
	  }
	]
  }
`)

var docOldExample = []byte(`
[ {
  "id": "73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c",
  "blobs": [
	{
	  "id": "3ec79977ef0cf5de7b08cd12b874cd0f62bbaf7f07f3497a5b1bbcc8cb39b1ce",
	  "type": "data",
	  "offset": 0,
	  "length": 38
	},{
	  "id": "9ccb846e60d90d4eb915848add7aa7ea1e4bbabfc60e573db9f7bfb2789afbae",
	  "type": "tree",
	  "offset": 38,
	  "length": 112
	},
	{
	  "id": "d3dc577b4ffd38cc4b32122cabf8655a0223ed22edfd93b353dc0c3f2b0fdf66",
	  "type": "data",
	  "offset": 150,
	  "length": 123
	}
  ]
} ]
`)

var exampleTests = []struct {
	id, packID         restic.ID
	tpe                restic.BlobType
	offset, length     uint
	uncompressedLength uint
}{
	{
		restic.TestParseID("3ec79977ef0cf5de7b08cd12b874cd0f62bbaf7f07f3497a5b1bbcc8cb39b1ce"),
		restic.TestParseID("73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c"),
		restic.DataBlob, 0, 38, 0,
	}, {
		restic.TestParseID("9ccb846e60d90d4eb915848add7aa7ea1e4bbabfc60e573db9f7bfb2789afbae"),
		restic.TestParseID("73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c"),
		restic.TreeBlob, 38, 112, 511,
	}, {
		restic.TestParseID("d3dc577b4ffd38cc4b32122cabf8655a0223ed22edfd93b353dc0c3f2b0fdf66"),
		restic.TestParseID("73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c"),
		restic.DataBlob, 150, 123, 234,
	},
}

var exampleLookupTest = struct {
	packID restic.ID
	blobs  map[restic.ID]restic.BlobType
}{
	restic.TestParseID("73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c"),
	map[restic.ID]restic.BlobType{
		restic.TestParseID("3ec79977ef0cf5de7b08cd12b874cd0f62bbaf7f07f3497a5b1bbcc8cb39b1ce"): restic.DataBlob,
		restic.TestParseID("9ccb846e60d90d4eb915848add7aa7ea1e4bbabfc60e573db9f7bfb2789afbae"): restic.TreeBlob,
		restic.TestParseID("d3dc577b4ffd38cc4b32122cabf8655a0223ed22edfd93b353dc0c3f2b0fdf66"): restic.DataBlob,
	},
}

func TestIndexUnserialize(t *testing.T) {
	for _, task := range []struct {
		idxBytes []byte
		version  int
	}{
		{docExampleV1, 1},
		{docExampleV2, 2},
	} {
		oldIdx := restic.IDs{restic.TestParseID("ed54ae36197f4745ebc4b54d10e0f623eaaaedd03013eb7ae90df881b7781452")}

		idx, oldFormat, err := index.DecodeIndex(task.idxBytes, restic.NewRandomID())
		rtest.OK(t, err)
		rtest.Assert(t, !oldFormat, "new index format recognized as old format")

		for _, test := range exampleTests {
			list := idx.Lookup(restic.BlobHandle{ID: test.id, Type: test.tpe}, nil)
			if len(list) != 1 {
				t.Errorf("expected one result for blob %v, got %v: %v", test.id.Str(), len(list), list)
			}
			blob := list[0]

			t.Logf("looking for blob %v/%v, got %v", test.tpe, test.id.Str(), blob)

			rtest.Equals(t, test.packID, blob.PackID)
			rtest.Equals(t, test.tpe, blob.Type)
			rtest.Equals(t, test.offset, blob.Offset)
			rtest.Equals(t, test.length, blob.Length)
			if task.version == 1 {
				rtest.Equals(t, uint(0), blob.UncompressedLength)
			} else if task.version == 2 {
				rtest.Equals(t, test.uncompressedLength, blob.UncompressedLength)
			} else {
				t.Fatal("Invalid index version")
			}
		}

		rtest.Equals(t, oldIdx, idx.Supersedes())

		blobs := listPack(idx, exampleLookupTest.packID)
		if len(blobs) != len(exampleLookupTest.blobs) {
			t.Fatalf("expected %d blobs in pack, got %d", len(exampleLookupTest.blobs), len(blobs))
		}

		for _, blob := range blobs {
			b, ok := exampleLookupTest.blobs[blob.ID]
			if !ok {
				t.Errorf("unexpected blob %v found", blob.ID.Str())
			}
			if blob.Type != b {
				t.Errorf("unexpected type for blob %v: want %v, got %v", blob.ID.Str(), b, blob.Type)
			}
		}
	}
}

func listPack(idx *index.Index, id restic.ID) (pbs []restic.PackedBlob) {
	idx.Each(context.TODO(), func(pb restic.PackedBlob) {
		if pb.PackID.Equal(id) {
			pbs = append(pbs, pb)
		}
	})
	return pbs
}

var (
	benchmarkIndexJSON     []byte
	benchmarkIndexJSONOnce sync.Once
)

func initBenchmarkIndexJSON() {
	idx, _ := createRandomIndex(rand.New(rand.NewSource(0)), 200000)
	var buf bytes.Buffer
	err := idx.Encode(&buf)
	if err != nil {
		panic(err)
	}

	benchmarkIndexJSON = buf.Bytes()
}

func BenchmarkDecodeIndex(b *testing.B) {
	benchmarkIndexJSONOnce.Do(initBenchmarkIndexJSON)

	id := restic.NewRandomID()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, err := index.DecodeIndex(benchmarkIndexJSON, id)
		rtest.OK(b, err)
	}
}

func BenchmarkDecodeIndexParallel(b *testing.B) {
	benchmarkIndexJSONOnce.Do(initBenchmarkIndexJSON)
	id := restic.NewRandomID()

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _, err := index.DecodeIndex(benchmarkIndexJSON, id)
			rtest.OK(b, err)
		}
	})
}

func BenchmarkEncodeIndex(b *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		idx, _ := createRandomIndex(rand.New(rand.NewSource(0)), n)

		b.Run(fmt.Sprint(n), func(b *testing.B) {
			buf := new(bytes.Buffer)
			err := idx.Encode(buf)
			rtest.OK(b, err)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				buf.Reset()
				_ = idx.Encode(buf)
			}
		})
	}
}

func TestIndexUnserializeOld(t *testing.T) {
	idx, oldFormat, err := index.DecodeIndex(docOldExample, restic.NewRandomID())
	rtest.OK(t, err)
	rtest.Assert(t, oldFormat, "old index format recognized as new format")

	for _, test := range exampleTests {
		list := idx.Lookup(restic.BlobHandle{ID: test.id, Type: test.tpe}, nil)
		if len(list) != 1 {
			t.Errorf("expected one result for blob %v, got %v: %v", test.id.Str(), len(list), list)
		}
		blob := list[0]

		rtest.Equals(t, test.packID, blob.PackID)
		rtest.Equals(t, test.tpe, blob.Type)
		rtest.Equals(t, test.offset, blob.Offset)
		rtest.Equals(t, test.length, blob.Length)
	}

	rtest.Equals(t, 0, len(idx.Supersedes()))
}

func TestIndexPacks(t *testing.T) {
	idx := index.NewIndex()
	packs := restic.NewIDSet()

	for i := 0; i < 20; i++ {
		packID := restic.NewRandomID()
		idx.StorePack(packID, []restic.Blob{
			{
				BlobHandle: restic.NewRandomBlobHandle(),
				Offset:     0,
				Length:     23,
			},
		})

		packs.Insert(packID)
	}

	idxPacks := idx.Packs()
	rtest.Assert(t, packs.Equals(idxPacks), "packs in index do not match packs added to index")
}

const maxPackSize = 16 * 1024 * 1024

// This function generates a (insecure) random ID, similar to NewRandomID
func NewRandomTestID(rng *rand.Rand) restic.ID {
	id := restic.ID{}
	rng.Read(id[:])
	return id
}

func createRandomIndex(rng *rand.Rand, packfiles int) (idx *index.Index, lookupBh restic.BlobHandle) {
	idx = index.NewIndex()

	// create index with given number of pack files
	for i := 0; i < packfiles; i++ {
		packID := NewRandomTestID(rng)
		var blobs []restic.Blob
		offset := 0
		for offset < maxPackSize {
			size := 2000 + rng.Intn(4*1024*1024)
			id := NewRandomTestID(rng)
			blobs = append(blobs, restic.Blob{
				BlobHandle: restic.BlobHandle{
					Type: restic.DataBlob,
					ID:   id,
				},
				Length:             uint(size),
				UncompressedLength: uint(2 * size),
				Offset:             uint(offset),
			})

			offset += size
		}
		idx.StorePack(packID, blobs)

		if i == 0 {
			lookupBh = restic.BlobHandle{
				Type: restic.DataBlob,
				ID:   blobs[rng.Intn(len(blobs))].ID,
			}
		}
	}

	return idx, lookupBh
}

func BenchmarkIndexHasUnknown(b *testing.B) {
	idx, _ := createRandomIndex(rand.New(rand.NewSource(0)), 200000)
	lookupBh := restic.NewRandomBlobHandle()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		idx.Has(lookupBh)
	}
}

func BenchmarkIndexHasKnown(b *testing.B) {
	idx, lookupBh := createRandomIndex(rand.New(rand.NewSource(0)), 200000)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		idx.Has(lookupBh)
	}
}

func BenchmarkIndexAlloc(b *testing.B) {
	rng := rand.New(rand.NewSource(0))
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		createRandomIndex(rng, 200000)
	}
}

func BenchmarkIndexAllocParallel(b *testing.B) {
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		rng := rand.New(rand.NewSource(0))
		for pb.Next() {
			createRandomIndex(rng, 200000)
		}
	})
}

func TestIndexHas(t *testing.T) {
	tests := []restic.PackedBlob{}

	idx := index.NewIndex()

	// create 50 packs with 20 blobs each
	for i := 0; i < 50; i++ {
		packID := restic.NewRandomID()
		var blobs []restic.Blob

		pos := uint(0)
		for j := 0; j < 20; j++ {
			length := uint(i*100 + j)
			uncompressedLength := uint(0)
			if i >= 25 {
				// test a mix of compressed and uncompressed packs
				uncompressedLength = 2 * length
			}
			pb := restic.PackedBlob{
				Blob: restic.Blob{
					BlobHandle:         restic.NewRandomBlobHandle(),
					Offset:             pos,
					Length:             length,
					UncompressedLength: uncompressedLength,
				},
				PackID: packID,
			}
			blobs = append(blobs, pb.Blob)
			tests = append(tests, pb)
			pos += length
		}
		idx.StorePack(packID, blobs)
	}

	for _, testBlob := range tests {
		rtest.Assert(t, idx.Has(testBlob.BlobHandle), "Index reports not having data blob added to it")
	}

	rtest.Assert(t, !idx.Has(restic.NewRandomBlobHandle()), "Index reports having a data blob not added to it")
	rtest.Assert(t, !idx.Has(restic.BlobHandle{ID: tests[0].ID, Type: restic.TreeBlob}), "Index reports having a tree blob added to it with the same id as a data blob")
}

func TestMixedEachByPack(t *testing.T) {
	idx := index.NewIndex()

	expected := make(map[restic.ID]int)
	// create 50 packs with 2 blobs each
	for i := 0; i < 50; i++ {
		packID := restic.NewRandomID()
		expected[packID] = 1
		blobs := []restic.Blob{
			{
				BlobHandle: restic.BlobHandle{Type: restic.DataBlob, ID: restic.NewRandomID()},
				Offset:     0,
				Length:     42,
			},
			{
				BlobHandle: restic.BlobHandle{Type: restic.TreeBlob, ID: restic.NewRandomID()},
				Offset:     42,
				Length:     43,
			},
		}
		idx.StorePack(packID, blobs)
	}

	reported := make(map[restic.ID]int)
	for bp := range idx.EachByPack(context.TODO(), restic.NewIDSet()) {
		reported[bp.PackID]++

		rtest.Equals(t, 2, len(bp.Blobs)) // correct blob count
		if bp.Blobs[0].Offset > bp.Blobs[1].Offset {
			bp.Blobs[1], bp.Blobs[0] = bp.Blobs[0], bp.Blobs[1]
		}
		b0 := bp.Blobs[0]
		rtest.Assert(t, b0.Type == restic.DataBlob && b0.Offset == 0 && b0.Length == 42, "wrong blob", b0)
		b1 := bp.Blobs[1]
		rtest.Assert(t, b1.Type == restic.TreeBlob && b1.Offset == 42 && b1.Length == 43, "wrong blob", b1)
	}
	rtest.Equals(t, expected, reported)
}

func TestEachByPackIgnoes(t *testing.T) {
	idx := index.NewIndex()

	ignores := restic.NewIDSet()
	expected := make(map[restic.ID]int)
	// create 50 packs with one blob each
	for i := 0; i < 50; i++ {
		packID := restic.NewRandomID()
		if i < 3 {
			ignores.Insert(packID)
		} else {
			expected[packID] = 1
		}
		blobs := []restic.Blob{
			{
				BlobHandle: restic.BlobHandle{Type: restic.DataBlob, ID: restic.NewRandomID()},
				Offset:     0,
				Length:     42,
			},
		}
		idx.StorePack(packID, blobs)
	}
	idx.Finalize()

	reported := make(map[restic.ID]int)
	for bp := range idx.EachByPack(context.TODO(), ignores) {
		reported[bp.PackID]++
		rtest.Equals(t, 1, len(bp.Blobs)) // correct blob count
		b0 := bp.Blobs[0]
		rtest.Assert(t, b0.Type == restic.DataBlob && b0.Offset == 0 && b0.Length == 42, "wrong blob", b0)
	}
	rtest.Equals(t, expected, reported)
}
