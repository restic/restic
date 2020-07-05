package repository_test

import (
	"bytes"
	"math/rand"
	"sync"
	"testing"

	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func TestIndexSerialize(t *testing.T) {
	type testEntry struct {
		id             restic.ID
		pack           restic.ID
		tpe            restic.BlobType
		offset, length uint
	}
	tests := []testEntry{}

	idx := repository.NewIndex()

	// create 50 packs with 20 blobs each
	for i := 0; i < 50; i++ {
		packID := restic.NewRandomID()

		pos := uint(0)
		for j := 0; j < 20; j++ {
			id := restic.NewRandomID()
			length := uint(i*100 + j)
			idx.Store(restic.PackedBlob{
				Blob: restic.Blob{
					Type:   restic.DataBlob,
					ID:     id,
					Offset: pos,
					Length: length,
				},
				PackID: packID,
			})

			tests = append(tests, testEntry{
				id:     id,
				pack:   packID,
				tpe:    restic.DataBlob,
				offset: pos,
				length: length,
			})

			pos += length
		}
	}

	wr := bytes.NewBuffer(nil)
	err := idx.Encode(wr)
	rtest.OK(t, err)

	idx2, err := repository.DecodeIndex(wr.Bytes())
	rtest.OK(t, err)
	rtest.Assert(t, idx2 != nil,
		"nil returned for decoded index")

	wr2 := bytes.NewBuffer(nil)
	err = idx2.Encode(wr2)
	rtest.OK(t, err)

	for _, testBlob := range tests {
		list, found := idx.Lookup(testBlob.id, testBlob.tpe)
		rtest.Assert(t, found, "Expected to find blob id %v", testBlob.id.Str())

		if len(list) != 1 {
			t.Errorf("expected one result for blob %v, got %v: %v", testBlob.id.Str(), len(list), list)
		}
		result := list[0]

		rtest.Equals(t, testBlob.pack, result.PackID)
		rtest.Equals(t, testBlob.tpe, result.Type)
		rtest.Equals(t, testBlob.offset, result.Offset)
		rtest.Equals(t, testBlob.length, result.Length)

		list2, found := idx2.Lookup(testBlob.id, testBlob.tpe)
		rtest.Assert(t, found, "Expected to find blob id %v", testBlob.id)

		if len(list2) != 1 {
			t.Errorf("expected one result for blob %v, got %v: %v", testBlob.id.Str(), len(list2), list2)
		}
		result2 := list2[0]

		rtest.Equals(t, testBlob.pack, result2.PackID)
		rtest.Equals(t, testBlob.tpe, result2.Type)
		rtest.Equals(t, testBlob.offset, result2.Offset)
		rtest.Equals(t, testBlob.length, result2.Length)
	}

	// add more blobs to idx
	newtests := []testEntry{}
	for i := 0; i < 10; i++ {
		packID := restic.NewRandomID()

		pos := uint(0)
		for j := 0; j < 10; j++ {
			id := restic.NewRandomID()
			length := uint(i*100 + j)
			idx.Store(restic.PackedBlob{
				Blob: restic.Blob{
					Type:   restic.DataBlob,
					ID:     id,
					Offset: pos,
					Length: length,
				},
				PackID: packID,
			})

			newtests = append(newtests, testEntry{
				id:     id,
				pack:   packID,
				tpe:    restic.DataBlob,
				offset: pos,
				length: length,
			})

			pos += length
		}
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
	id2, err := idx.ID()
	rtest.OK(t, err)
	rtest.Assert(t, id2.Equal(id),
		"wrong ID returned: want %v, got %v", id, id2)

	idx3, err := repository.DecodeIndex(wr3.Bytes())
	rtest.OK(t, err)
	rtest.Assert(t, idx3 != nil,
		"nil returned for decoded index")
	rtest.Assert(t, idx3.Final(),
		"decoded index is not final")

	// all new blobs must be in the index
	for _, testBlob := range newtests {
		list, found := idx3.Lookup(testBlob.id, testBlob.tpe)
		rtest.Assert(t, found, "Expected to find blob id %v", testBlob.id.Str())

		if len(list) != 1 {
			t.Errorf("expected one result for blob %v, got %v: %v", testBlob.id.Str(), len(list), list)
		}

		blob := list[0]

		rtest.Equals(t, testBlob.pack, blob.PackID)
		rtest.Equals(t, testBlob.tpe, blob.Type)
		rtest.Equals(t, testBlob.offset, blob.Offset)
		rtest.Equals(t, testBlob.length, blob.Length)
	}
}

func TestIndexSize(t *testing.T) {
	idx := repository.NewIndex()

	packs := 200
	blobs := 100
	for i := 0; i < packs; i++ {
		packID := restic.NewRandomID()

		pos := uint(0)
		for j := 0; j < blobs; j++ {
			id := restic.NewRandomID()
			length := uint(i*100 + j)
			idx.Store(restic.PackedBlob{
				Blob: restic.Blob{
					Type:   restic.DataBlob,
					ID:     id,
					Offset: pos,
					Length: length,
				},
				PackID: packID,
			})

			pos += length
		}
	}

	wr := bytes.NewBuffer(nil)

	err := idx.Encode(wr)
	rtest.OK(t, err)

	t.Logf("Index file size for %d blobs in %d packs is %d", blobs*packs, packs, wr.Len())
}

// example index serialization from doc/Design.rst
var docExample = []byte(`
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
		  "length": 25
		},{
		  "id": "9ccb846e60d90d4eb915848add7aa7ea1e4bbabfc60e573db9f7bfb2789afbae",
		  "type": "tree",
		  "offset": 38,
		  "length": 100
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

var docOldExample = []byte(`
[ {
  "id": "73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c",
  "blobs": [
	{
	  "id": "3ec79977ef0cf5de7b08cd12b874cd0f62bbaf7f07f3497a5b1bbcc8cb39b1ce",
	  "type": "data",
	  "offset": 0,
	  "length": 25
	},{
	  "id": "9ccb846e60d90d4eb915848add7aa7ea1e4bbabfc60e573db9f7bfb2789afbae",
	  "type": "tree",
	  "offset": 38,
	  "length": 100
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
	id, packID     restic.ID
	tpe            restic.BlobType
	offset, length uint
}{
	{
		restic.TestParseID("3ec79977ef0cf5de7b08cd12b874cd0f62bbaf7f07f3497a5b1bbcc8cb39b1ce"),
		restic.TestParseID("73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c"),
		restic.DataBlob, 0, 25,
	}, {
		restic.TestParseID("9ccb846e60d90d4eb915848add7aa7ea1e4bbabfc60e573db9f7bfb2789afbae"),
		restic.TestParseID("73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c"),
		restic.TreeBlob, 38, 100,
	}, {
		restic.TestParseID("d3dc577b4ffd38cc4b32122cabf8655a0223ed22edfd93b353dc0c3f2b0fdf66"),
		restic.TestParseID("73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c"),
		restic.DataBlob, 150, 123,
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
	oldIdx := restic.IDs{restic.TestParseID("ed54ae36197f4745ebc4b54d10e0f623eaaaedd03013eb7ae90df881b7781452")}

	idx, err := repository.DecodeIndex(docExample)
	rtest.OK(t, err)

	for _, test := range exampleTests {
		list, found := idx.Lookup(test.id, test.tpe)
		rtest.Assert(t, found, "Expected to find blob id %v", test.id.Str())

		if len(list) != 1 {
			t.Errorf("expected one result for blob %v, got %v: %v", test.id.Str(), len(list), list)
		}
		blob := list[0]

		t.Logf("looking for blob %v/%v, got %v", test.tpe, test.id.Str(), blob)

		rtest.Equals(t, test.packID, blob.PackID)
		rtest.Equals(t, test.tpe, blob.Type)
		rtest.Equals(t, test.offset, blob.Offset)
		rtest.Equals(t, test.length, blob.Length)
	}

	rtest.Equals(t, oldIdx, idx.Supersedes())

	blobs := idx.ListPack(exampleLookupTest.packID)
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

var (
	benchmarkIndexJSON     []byte
	benchmarkIndexJSONOnce sync.Once
)

func initBenchmarkIndexJSON() {
	idx, _ := createRandomIndex(rand.New(rand.NewSource(0)))
	var buf bytes.Buffer
	idx.Encode(&buf)
	benchmarkIndexJSON = buf.Bytes()
}

func BenchmarkDecodeIndex(b *testing.B) {
	benchmarkIndexJSONOnce.Do(initBenchmarkIndexJSON)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := repository.DecodeIndex(benchmarkIndexJSON)
		rtest.OK(b, err)
	}
}

func BenchmarkDecodeIndexParallel(b *testing.B) {
	benchmarkIndexJSONOnce.Do(initBenchmarkIndexJSON)
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := repository.DecodeIndex(benchmarkIndexJSON)
			rtest.OK(b, err)
		}
	})
}

func TestIndexUnserializeOld(t *testing.T) {
	idx, err := repository.DecodeOldIndex(docOldExample)
	rtest.OK(t, err)

	for _, test := range exampleTests {
		list, found := idx.Lookup(test.id, test.tpe)
		rtest.Assert(t, found, "Expected to find blob id %v", test.id.Str())

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
	idx := repository.NewIndex()
	packs := restic.NewIDSet()

	for i := 0; i < 20; i++ {
		packID := restic.NewRandomID()
		idx.Store(restic.PackedBlob{
			Blob: restic.Blob{
				Type:   restic.DataBlob,
				ID:     restic.NewRandomID(),
				Offset: 0,
				Length: 23,
			},
			PackID: packID,
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

func createRandomIndex(rng *rand.Rand) (idx *repository.Index, lookupID restic.ID) {
	idx = repository.NewIndex()

	// create index with 200k pack files
	for i := 0; i < 200000; i++ {
		packID := NewRandomTestID(rng)
		var blobs []restic.Blob
		offset := 0
		for offset < maxPackSize {
			size := 2000 + rng.Intn(4*1024*1024)
			id := NewRandomTestID(rng)
			blobs = append(blobs, restic.Blob{
				Type:   restic.DataBlob,
				ID:     id,
				Length: uint(size),
				Offset: uint(offset),
			})

			offset += size
		}
		idx.StorePack(packID, blobs)

		if i == 0 {
			lookupID = blobs[rng.Intn(len(blobs))].ID
		}
	}

	return idx, lookupID
}

func BenchmarkIndexHasUnknown(b *testing.B) {
	idx, _ := createRandomIndex(rand.New(rand.NewSource(0)))
	lookupID := restic.NewRandomID()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		idx.Has(lookupID, restic.DataBlob)
	}
}

func BenchmarkIndexHasKnown(b *testing.B) {
	idx, lookupID := createRandomIndex(rand.New(rand.NewSource(0)))

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		idx.Has(lookupID, restic.DataBlob)
	}
}

func BenchmarkIndexAlloc(b *testing.B) {
	rng := rand.New(rand.NewSource(0))
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		createRandomIndex(rng)
	}
}

func BenchmarkIndexAllocParallel(b *testing.B) {
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		rng := rand.New(rand.NewSource(0))
		for pb.Next() {
			createRandomIndex(rng)
		}
	})
}

func TestIndexHas(t *testing.T) {
	type testEntry struct {
		id             restic.ID
		pack           restic.ID
		tpe            restic.BlobType
		offset, length uint
	}
	tests := []testEntry{}

	idx := repository.NewIndex()

	// create 50 packs with 20 blobs each
	for i := 0; i < 50; i++ {
		packID := restic.NewRandomID()

		pos := uint(0)
		for j := 0; j < 20; j++ {
			id := restic.NewRandomID()
			length := uint(i*100 + j)
			idx.Store(restic.PackedBlob{
				Blob: restic.Blob{
					Type:   restic.DataBlob,
					ID:     id,
					Offset: pos,
					Length: length,
				},
				PackID: packID,
			})

			tests = append(tests, testEntry{
				id:     id,
				pack:   packID,
				tpe:    restic.DataBlob,
				offset: pos,
				length: length,
			})

			pos += length
		}
	}

	for _, testBlob := range tests {
		rtest.Assert(t, idx.Has(testBlob.id, testBlob.tpe), "Index reports not having data blob added to it")
	}

	rtest.Assert(t, !idx.Has(restic.NewRandomID(), restic.DataBlob), "Index reports having a data blob not added to it")
	rtest.Assert(t, !idx.Has(tests[0].id, restic.TreeBlob), "Index reports having a tree blob added to it with the same id as a data blob")
}
