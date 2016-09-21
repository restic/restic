package index

import (
	"math/rand"
	"restic"
	"restic/repository"
	"testing"
	"time"
)

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

func validateIndex(t testing.TB, repo restic.Repository, idx *Index) {
	for id := range repo.List(restic.DataFile, nil) {
		if _, ok := idx.Packs[id]; !ok {
			t.Errorf("pack %v missing from index", id.Str())
		}
	}
}

func TestIndexNew(t *testing.T) {
	repo, cleanup := createFilledRepo(t, 3, 0)
	defer cleanup()

	idx, err := New(repo, nil)
	if err != nil {
		t.Fatalf("New() returned error %v", err)
	}

	if idx == nil {
		t.Fatalf("New() returned nil index")
	}

	validateIndex(t, repo, idx)
}

func TestIndexLoad(t *testing.T) {
	repo, cleanup := createFilledRepo(t, 3, 0)
	defer cleanup()

	loadIdx, err := Load(repo, nil)
	if err != nil {
		t.Fatalf("Load() returned error %v", err)
	}

	if loadIdx == nil {
		t.Fatalf("Load() returned nil index")
	}

	validateIndex(t, repo, loadIdx)

	newIdx, err := New(repo, nil)
	if err != nil {
		t.Fatalf("New() returned error %v", err)
	}

	if len(loadIdx.Packs) != len(newIdx.Packs) {
		t.Errorf("number of packs does not match: want %v, got %v",
			len(loadIdx.Packs), len(newIdx.Packs))
	}

	validateIndex(t, repo, newIdx)

	for packID, packNew := range newIdx.Packs {
		packLoad, ok := loadIdx.Packs[packID]

		if !ok {
			t.Errorf("loaded index does not list pack %v", packID.Str())
			continue
		}

		if len(packNew.Entries) != len(packLoad.Entries) {
			t.Errorf("  number of entries in pack %v does not match: %d != %d\n  %v\n  %v",
				packID.Str(), len(packNew.Entries), len(packLoad.Entries),
				packNew.Entries, packLoad.Entries)
			continue
		}

		for _, entryNew := range packNew.Entries {
			found := false
			for _, entryLoad := range packLoad.Entries {
				if !entryLoad.ID.Equal(entryNew.ID) {
					continue
				}

				if entryLoad.Type != entryNew.Type {
					continue
				}

				if entryLoad.Offset != entryNew.Offset {
					continue
				}

				if entryLoad.Length != entryNew.Length {
					continue
				}

				found = true
				break
			}

			if !found {
				t.Errorf("blob not found in loaded index: %v", entryNew)
			}
		}
	}
}

func BenchmarkIndexNew(b *testing.B) {
	repo, cleanup := createFilledRepo(b, 3, 0)
	defer cleanup()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		idx, err := New(repo, nil)

		if err != nil {
			b.Fatalf("New() returned error %v", err)
		}

		if idx == nil {
			b.Fatalf("New() returned nil index")
		}
	}
}

func TestIndexDuplicateBlobs(t *testing.T) {
	repo, cleanup := createFilledRepo(t, 3, 0.01)
	defer cleanup()

	idx, err := New(repo, nil)
	if err != nil {
		t.Fatal(err)
	}

	dups := idx.DuplicateBlobs()
	if len(dups) == 0 {
		t.Errorf("no duplicate blobs found")
	}
	t.Logf("%d packs, %d unique blobs", len(idx.Packs), len(idx.Blobs))

	packs := idx.PacksForBlobs(dups)
	if len(packs) == 0 {
		t.Errorf("no packs with duplicate blobs found")
	}
	t.Logf("%d packs with duplicate blobs", len(packs))
}

func loadIndex(t testing.TB, repo restic.Repository) *Index {
	idx, err := Load(repo, nil)
	if err != nil {
		t.Fatalf("Load() returned error %v", err)
	}

	return idx
}

func TestIndexSave(t *testing.T) {
	repo, cleanup := createFilledRepo(t, 3, 0)
	defer cleanup()

	idx := loadIndex(t, repo)

	packs := make(map[restic.ID][]restic.Blob)
	for id := range idx.Packs {
		if rand.Float32() < 0.5 {
			packs[id] = idx.Packs[id].Entries
		}
	}

	t.Logf("save %d/%d packs in a new index\n", len(packs), len(idx.Packs))

	id, err := Save(repo, packs, idx.IndexIDs.List())
	if err != nil {
		t.Fatalf("unable to save new index: %v", err)
	}

	t.Logf("new index saved as %v", id.Str())

	for id := range idx.IndexIDs {
		t.Logf("remove index %v", id.Str())
		err = repo.Backend().Remove(restic.IndexFile, id.String())
		if err != nil {
			t.Errorf("error removing index %v: %v", id, err)
		}
	}

	idx2 := loadIndex(t, repo)
	t.Logf("load new index with %d packs", len(idx2.Packs))

	if len(idx2.Packs) != len(packs) {
		t.Errorf("wrong number of packs in new index, want %d, got %d", len(packs), len(idx2.Packs))
	}

	for id := range packs {
		if _, ok := idx2.Packs[id]; !ok {
			t.Errorf("pack %v is not contained in new index", id.Str())
		}
	}

	for id := range idx2.Packs {
		if _, ok := packs[id]; !ok {
			t.Errorf("pack %v is not contained in new index", id.Str())
		}
	}
}

func TestIndexAddRemovePack(t *testing.T) {
	repo, cleanup := createFilledRepo(t, 3, 0)
	defer cleanup()

	idx, err := Load(repo, nil)
	if err != nil {
		t.Fatalf("Load() returned error %v", err)
	}

	done := make(chan struct{})
	defer close(done)

	packID := <-repo.List(restic.DataFile, done)

	t.Logf("selected pack %v", packID.Str())

	blobs := idx.Packs[packID].Entries

	idx.RemovePack(packID)

	if _, ok := idx.Packs[packID]; ok {
		t.Errorf("removed pack %v found in index.Packs", packID.Str())
	}

	for _, blob := range blobs {
		h := restic.BlobHandle{ID: blob.ID, Type: blob.Type}
		_, err := idx.FindBlob(h)
		if err == nil {
			t.Errorf("removed blob %v found in index", h)
		}

		if _, ok := idx.Blobs[h]; ok {
			t.Errorf("removed blob %v found in index.Blobs", h)
		}
	}

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

func TestIndexLoadDocReference(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	id, err := repo.SaveUnpacked(restic.IndexFile, docExample)
	if err != nil {
		t.Fatalf("SaveUnpacked() returned error %v", err)
	}

	t.Logf("index saved as %v", id.Str())

	idx := loadIndex(t, repo)

	blobID := restic.TestParseID("d3dc577b4ffd38cc4b32122cabf8655a0223ed22edfd93b353dc0c3f2b0fdf66")
	locs, err := idx.FindBlob(restic.BlobHandle{ID: blobID, Type: restic.DataBlob})
	if err != nil {
		t.Errorf("FindBlob() returned error %v", err)
	}

	if len(locs) != 1 {
		t.Errorf("blob found %d times, expected just one", len(locs))
	}

	l := locs[0]
	if !l.ID.Equal(blobID) {
		t.Errorf("blob IDs are not equal: %v != %v", l.ID, blobID)
	}

	if l.Type != restic.DataBlob {
		t.Errorf("want type %v, got %v", restic.DataBlob, l.Type)
	}

	if l.Offset != 150 {
		t.Errorf("wrong offset, want %d, got %v", 150, l.Offset)
	}

	if l.Length != 123 {
		t.Errorf("wrong length, want %d, got %v", 123, l.Length)
	}
}
