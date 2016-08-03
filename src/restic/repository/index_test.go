package repository_test

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"

	"restic/backend"
	"restic/pack"
	"restic/repository"
	. "restic/test"
)

func randomID() backend.ID {
	id := backend.ID{}
	_, err := io.ReadFull(rand.Reader, id[:])
	if err != nil {
		panic(err)
	}
	return id
}

func TestIndexSerialize(t *testing.T) {
	type testEntry struct {
		id             backend.ID
		pack           backend.ID
		tpe            pack.BlobType
		offset, length uint
	}
	tests := []testEntry{}

	idx := repository.NewIndex()

	// create 50 packs with 20 blobs each
	for i := 0; i < 50; i++ {
		packID := randomID()

		pos := uint(0)
		for j := 0; j < 20; j++ {
			id := randomID()
			length := uint(i*100 + j)
			idx.Store(repository.PackedBlob{
				Type:   pack.Data,
				ID:     id,
				PackID: packID,
				Offset: pos,
				Length: length,
			})

			tests = append(tests, testEntry{
				id:     id,
				pack:   packID,
				tpe:    pack.Data,
				offset: pos,
				length: length,
			})

			pos += length
		}
	}

	wr := bytes.NewBuffer(nil)
	err := idx.Encode(wr)
	OK(t, err)

	idx2, err := repository.DecodeIndex(wr)
	OK(t, err)
	Assert(t, idx2 != nil,
		"nil returned for decoded index")

	wr2 := bytes.NewBuffer(nil)
	err = idx2.Encode(wr2)
	OK(t, err)

	for _, testBlob := range tests {
		list, err := idx.Lookup(testBlob.id, testBlob.tpe)
		OK(t, err)

		if len(list) != 1 {
			t.Errorf("expected one result for blob %v, got %v: %v", testBlob.id.Str(), len(list), list)
		}
		result := list[0]

		Equals(t, testBlob.pack, result.PackID)
		Equals(t, testBlob.tpe, result.Type)
		Equals(t, testBlob.offset, result.Offset)
		Equals(t, testBlob.length, result.Length)

		list2, err := idx2.Lookup(testBlob.id, testBlob.tpe)
		OK(t, err)

		if len(list2) != 1 {
			t.Errorf("expected one result for blob %v, got %v: %v", testBlob.id.Str(), len(list2), list2)
		}
		result2 := list2[0]

		Equals(t, testBlob.pack, result2.PackID)
		Equals(t, testBlob.tpe, result2.Type)
		Equals(t, testBlob.offset, result2.Offset)
		Equals(t, testBlob.length, result2.Length)
	}

	// add more blobs to idx
	newtests := []testEntry{}
	for i := 0; i < 10; i++ {
		packID := randomID()

		pos := uint(0)
		for j := 0; j < 10; j++ {
			id := randomID()
			length := uint(i*100 + j)
			idx.Store(repository.PackedBlob{
				Type:   pack.Data,
				ID:     id,
				PackID: packID,
				Offset: pos,
				Length: length,
			})

			newtests = append(newtests, testEntry{
				id:     id,
				pack:   packID,
				tpe:    pack.Data,
				offset: pos,
				length: length,
			})

			pos += length
		}
	}

	// serialize idx, unserialize to idx3
	wr3 := bytes.NewBuffer(nil)
	err = idx.Finalize(wr3)
	OK(t, err)

	Assert(t, idx.Final(),
		"index not final after encoding")

	id := randomID()
	OK(t, idx.SetID(id))
	id2, err := idx.ID()
	Assert(t, id2.Equal(id),
		"wrong ID returned: want %v, got %v", id, id2)

	idx3, err := repository.DecodeIndex(wr3)
	OK(t, err)
	Assert(t, idx3 != nil,
		"nil returned for decoded index")
	Assert(t, idx3.Final(),
		"decoded index is not final")

	// all new blobs must be in the index
	for _, testBlob := range newtests {
		list, err := idx3.Lookup(testBlob.id, testBlob.tpe)
		OK(t, err)

		if len(list) != 1 {
			t.Errorf("expected one result for blob %v, got %v: %v", testBlob.id.Str(), len(list), list)
		}

		blob := list[0]

		Equals(t, testBlob.pack, blob.PackID)
		Equals(t, testBlob.tpe, blob.Type)
		Equals(t, testBlob.offset, blob.Offset)
		Equals(t, testBlob.length, blob.Length)
	}
}

func TestIndexSize(t *testing.T) {
	idx := repository.NewIndex()

	packs := 200
	blobs := 100
	for i := 0; i < packs; i++ {
		packID := randomID()

		pos := uint(0)
		for j := 0; j < blobs; j++ {
			id := randomID()
			length := uint(i*100 + j)
			idx.Store(repository.PackedBlob{
				Type:   pack.Data,
				ID:     id,
				PackID: packID,
				Offset: pos,
				Length: length,
			})

			pos += length
		}
	}

	wr := bytes.NewBuffer(nil)

	err := idx.Encode(wr)
	OK(t, err)

	t.Logf("Index file size for %d blobs in %d packs is %d", blobs*packs, packs, wr.Len())
}

// example index serialization from doc/Design.md
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
	id, packID     backend.ID
	tpe            pack.BlobType
	offset, length uint
}{
	{
		ParseID("3ec79977ef0cf5de7b08cd12b874cd0f62bbaf7f07f3497a5b1bbcc8cb39b1ce"),
		ParseID("73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c"),
		pack.Data, 0, 25,
	}, {
		ParseID("9ccb846e60d90d4eb915848add7aa7ea1e4bbabfc60e573db9f7bfb2789afbae"),
		ParseID("73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c"),
		pack.Tree, 38, 100,
	}, {
		ParseID("d3dc577b4ffd38cc4b32122cabf8655a0223ed22edfd93b353dc0c3f2b0fdf66"),
		ParseID("73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c"),
		pack.Data, 150, 123,
	},
}

var exampleLookupTest = struct {
	packID backend.ID
	blobs  map[backend.ID]pack.BlobType
}{
	ParseID("73d04e6125cf3c28a299cc2f3cca3b78ceac396e4fcf9575e34536b26782413c"),
	map[backend.ID]pack.BlobType{
		ParseID("3ec79977ef0cf5de7b08cd12b874cd0f62bbaf7f07f3497a5b1bbcc8cb39b1ce"): pack.Data,
		ParseID("9ccb846e60d90d4eb915848add7aa7ea1e4bbabfc60e573db9f7bfb2789afbae"): pack.Tree,
		ParseID("d3dc577b4ffd38cc4b32122cabf8655a0223ed22edfd93b353dc0c3f2b0fdf66"): pack.Data,
	},
}

func TestIndexUnserialize(t *testing.T) {
	oldIdx := backend.IDs{ParseID("ed54ae36197f4745ebc4b54d10e0f623eaaaedd03013eb7ae90df881b7781452")}

	idx, err := repository.DecodeIndex(bytes.NewReader(docExample))
	OK(t, err)

	for _, test := range exampleTests {
		list, err := idx.Lookup(test.id, test.tpe)
		OK(t, err)

		if len(list) != 1 {
			t.Errorf("expected one result for blob %v, got %v: %v", test.id.Str(), len(list), list)
		}
		blob := list[0]

		t.Logf("looking for blob %v/%v, got %v", test.tpe, test.id.Str(), blob)

		Equals(t, test.packID, blob.PackID)
		Equals(t, test.tpe, blob.Type)
		Equals(t, test.offset, blob.Offset)
		Equals(t, test.length, blob.Length)
	}

	Equals(t, oldIdx, idx.Supersedes())

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

func TestIndexUnserializeOld(t *testing.T) {
	idx, err := repository.DecodeOldIndex(bytes.NewReader(docOldExample))
	OK(t, err)

	for _, test := range exampleTests {
		list, err := idx.Lookup(test.id, test.tpe)
		OK(t, err)

		if len(list) != 1 {
			t.Errorf("expected one result for blob %v, got %v: %v", test.id.Str(), len(list), list)
		}
		blob := list[0]

		Equals(t, test.packID, blob.PackID)
		Equals(t, test.tpe, blob.Type)
		Equals(t, test.offset, blob.Offset)
		Equals(t, test.length, blob.Length)
	}

	Equals(t, 0, len(idx.Supersedes()))
}

func TestIndexPacks(t *testing.T) {
	idx := repository.NewIndex()
	packs := backend.NewIDSet()

	for i := 0; i < 20; i++ {
		packID := randomID()
		idx.Store(repository.PackedBlob{
			Type:   pack.Data,
			ID:     randomID(),
			PackID: packID,
			Offset: 0,
			Length: 23,
		})

		packs.Insert(packID)
	}

	idxPacks := idx.Packs()
	Assert(t, packs.Equals(idxPacks), "packs in index do not match packs added to index")
}
