package repository_test

import (
	"bytes"
	"crypto/rand"
	"io"
	"path/filepath"
	"testing"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/pack"
	"github.com/restic/restic/repository"
	. "github.com/restic/restic/test"
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
			idx.Store(pack.Data, id, &packID, pos, length)

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
		packID, tpe, offset, length, err := idx.Lookup(testBlob.id)
		OK(t, err)

		Equals(t, testBlob.pack, *packID)
		Equals(t, testBlob.tpe, tpe)
		Equals(t, testBlob.offset, offset)
		Equals(t, testBlob.length, length)

		packID, tpe, offset, length, err = idx2.Lookup(testBlob.id)
		OK(t, err)

		Equals(t, testBlob.pack, *packID)
		Equals(t, testBlob.tpe, tpe)
		Equals(t, testBlob.offset, offset)
		Equals(t, testBlob.length, length)
	}

	// add more blobs to idx
	newtests := []testEntry{}
	for i := 0; i < 10; i++ {
		packID := randomID()

		pos := uint(0)
		for j := 0; j < 10; j++ {
			id := randomID()
			length := uint(i*100 + j)
			idx.Store(pack.Data, id, &packID, pos, length)

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

	idx3, err := repository.DecodeIndex(wr3)
	OK(t, err)
	Assert(t, idx3 != nil,
		"nil returned for decoded index")
	Assert(t, idx3.Final(),
		"decoded index is not final")

	// all new blobs must be in the index
	for _, testBlob := range newtests {
		packID, tpe, offset, length, err := idx3.Lookup(testBlob.id)
		OK(t, err)

		Equals(t, testBlob.pack, *packID)
		Equals(t, testBlob.tpe, tpe)
		Equals(t, testBlob.offset, offset)
		Equals(t, testBlob.length, length)
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
			idx.Store(pack.Data, id, &packID, pos, length)

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

func TestIndexUnserialize(t *testing.T) {
	oldIdx := backend.IDs{ParseID("ed54ae36197f4745ebc4b54d10e0f623eaaaedd03013eb7ae90df881b7781452")}

	idx, err := repository.DecodeIndex(bytes.NewReader(docExample))
	OK(t, err)

	for _, test := range exampleTests {
		packID, tpe, offset, length, err := idx.Lookup(test.id)
		OK(t, err)

		Equals(t, test.packID, *packID)
		Equals(t, test.tpe, tpe)
		Equals(t, test.offset, offset)
		Equals(t, test.length, length)
	}

	Equals(t, oldIdx, idx.Supersedes())
}

func TestIndexUnserializeOld(t *testing.T) {
	idx, err := repository.DecodeOldIndex(bytes.NewReader(docOldExample))
	OK(t, err)

	for _, test := range exampleTests {
		packID, tpe, offset, length, err := idx.Lookup(test.id)
		OK(t, err)

		Equals(t, test.packID, *packID)
		Equals(t, test.tpe, tpe)
		Equals(t, test.offset, offset)
		Equals(t, test.length, length)
	}

	Equals(t, 0, len(idx.Supersedes()))
}

var oldIndexTestRepo = filepath.Join("testdata", "old-index-repo.tar.gz")

func TestConvertIndex(t *testing.T) {
	WithTestEnvironment(t, oldIndexTestRepo, func(repodir string) {
		repo := OpenLocalRepo(t, repodir)

		old := make(map[backend.ID]*repository.Index)
		for id := range repo.List(backend.Index, nil) {
			idx, err := repository.LoadIndex(repo, id.String())
			OK(t, err)
			old[id] = idx
		}

		OK(t, repository.ConvertIndexes(repo))

		for id := range repo.List(backend.Index, nil) {
			idx, err := repository.LoadIndexWithDecoder(repo, id.String(), repository.DecodeIndex)
			OK(t, err)

			Assert(t, len(idx.Supersedes()) == 1,
				"Expected index %v to supersed exactly one index, got %v", id, idx.Supersedes())

			oldIndexID := idx.Supersedes()[0]

			oldIndex, ok := old[oldIndexID]
			Assert(t, ok,
				"Index %v superseds %v, but that wasn't found in the old index map", id.Str(), oldIndexID.Str())

			Assert(t, idx.Count(pack.Data) == oldIndex.Count(pack.Data),
				"Index %v count blobs %v: %v != %v", id.Str(), pack.Data, idx.Count(pack.Data), oldIndex.Count(pack.Data))
			Assert(t, idx.Count(pack.Tree) == oldIndex.Count(pack.Tree),
				"Index %v count blobs %v: %v != %v", id.Str(), pack.Tree, idx.Count(pack.Tree), oldIndex.Count(pack.Tree))

			for packedBlob := range idx.Each(nil) {
				packID, tpe, offset, length, err := oldIndex.Lookup(packedBlob.ID)
				OK(t, err)

				Assert(t, *packID == packedBlob.PackID,
					"Check blob %v: pack ID %v != %v", packedBlob.ID, packID, packedBlob.PackID)
				Assert(t, tpe == packedBlob.Type,
					"Check blob %v: Type %v != %v", packedBlob.ID, tpe, packedBlob.Type)
				Assert(t, offset == packedBlob.Offset,
					"Check blob %v: Type %v != %v", packedBlob.ID, offset, packedBlob.Offset)
				Assert(t, length == packedBlob.Length,
					"Check blob %v: Type %v != %v", packedBlob.ID, length, packedBlob.Length)
			}
		}
	})
}

func TestStoreOverwritesPreliminaryEntry(t *testing.T) {
	idx := repository.NewIndex()

	blobID := randomID()
	dataType := pack.Data
	err := idx.StoreInProgress(dataType, blobID)
	OK(t, err)

	packID := randomID()
	offset := uint(0)
	length := uint(100)
	idx.Store(dataType, blobID, &packID, offset, length)

	actPackID, actType, actOffset, actLength, err := idx.Lookup(blobID)
	OK(t, err)
	Equals(t, packID, *actPackID)
	Equals(t, dataType, actType)
	Equals(t, offset, actOffset)
	Equals(t, length, actLength)
}

func TestStoreInProgressAddsPreliminaryEntry(t *testing.T) {
	idx := repository.NewIndex()

	blobID := randomID()
	dataType := pack.Data

	err := idx.StoreInProgress(dataType, blobID)
	OK(t, err)

	actPackID, actType, actOffset, actLength, err := idx.Lookup(blobID)
	OK(t, err)
	Assert(t, actPackID == nil,
		"Preliminary index entry illegaly associated with a pack id.")
	Equals(t, uint(0), actOffset)
	Equals(t, uint(0), actLength)
	Equals(t, dataType, actType)
}

func TestStoreInProgressRefusesToOverwriteExistingFinalEntry(t *testing.T) {
	idx := repository.NewIndex()

	blobID := randomID()
	dataType := pack.Data
	packID := randomID()
	offset := uint(0)
	length := uint(100)
	idx.Store(dataType, blobID, &packID, offset, length)

	err := idx.StoreInProgress(dataType, blobID)
	Assert(t, err != nil,
		"index.StoreInProgress did not refuse to overwrite existing entry")
}

func TestStoreInProgressRefusesToOverwriteExistingPreliminaryEntry(t *testing.T) {
	idx := repository.NewIndex()

	blobID := randomID()
	dataType := pack.Data

	_ = idx.StoreInProgress(dataType, blobID)
	err := idx.StoreInProgress(dataType, blobID)
	Assert(t, err != nil,
		"index.StoreInProgress did not refuse to overwrite existing entry")
}
