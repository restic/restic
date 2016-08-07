package index

import (
	"restic"
	"restic/backend"
	"restic/backend/local"
	"restic/repository"
	"testing"
	"time"
)

var (
	snapshotTime = time.Unix(1470492820, 207401672)
	snapshots    = 3
	depth        = 3
)

func createFilledRepo(t testing.TB, snapshots int) (*repository.Repository, func()) {
	repo, cleanup := repository.TestRepository(t)

	for i := 0; i < 3; i++ {
		restic.TestCreateSnapshot(t, repo, snapshotTime.Add(time.Duration(i)*time.Second), depth)
	}

	return repo, cleanup
}

func validateIndex(t testing.TB, repo *repository.Repository, idx *Index) {
	for id := range repo.List(backend.Data, nil) {
		if _, ok := idx.Packs[id]; !ok {
			t.Errorf("pack %v missing from index", id.Str())
		}
	}
}

func TestIndexNew(t *testing.T) {
	repo, cleanup := createFilledRepo(t, 3)
	defer cleanup()

	idx, err := New(repo)
	if err != nil {
		t.Fatalf("New() returned error %v", err)
	}

	if idx == nil {
		t.Fatalf("New() returned nil index")
	}

	validateIndex(t, repo, idx)
}

func TestIndexLoad(t *testing.T) {
	repo, cleanup := createFilledRepo(t, 3)
	defer cleanup()

	loadIdx, err := Load(repo)
	if err != nil {
		t.Fatalf("Load() returned error %v", err)
	}

	if loadIdx == nil {
		t.Fatalf("Load() returned nil index")
	}

	validateIndex(t, repo, loadIdx)

	newIdx, err := New(repo)
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

func openRepo(t testing.TB, dir, password string) *repository.Repository {
	b, err := local.Open(dir)
	if err != nil {
		t.Fatalf("open backend %v failed: %v", dir, err)
	}

	r := repository.New(b)
	err = r.SearchKey(password)
	if err != nil {
		t.Fatalf("unable to open repo with password: %v", err)
	}

	return r
}

func BenchmarkIndexNew(b *testing.B) {
	repo, cleanup := createFilledRepo(b, 3)
	defer cleanup()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		idx, err := New(repo)

		if err != nil {
			b.Fatalf("New() returned error %v", err)
		}

		if idx == nil {
			b.Fatalf("New() returned nil index")
		}
	}
}
