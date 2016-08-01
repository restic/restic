package repository

import (
	"math/rand"
	"restic/backend"
	"restic/repository"
	"testing"
)

func repack(t *testing.T, repo *repository.Repository, packs, blobs backend.IDSet) {
	err := repository.Repack(repo, packs, blobs)
	if err != nil {
		t.Fatal(err)
	}
}

func saveIndex(t *testing.T, repo *repository.Repository) {
	if err := repo.SaveIndex(); err != nil {
		t.Fatalf("repo.SaveIndex() %v", err)
	}
}

func rebuildIndex(t *testing.T, repo *repository.Repository) {
	if err := repository.RebuildIndex(repo); err != nil {
		t.Fatalf("error rebuilding index: %v", err)
	}
}

func reloadIndex(t *testing.T, repo *repository.Repository) {
	repo.SetIndex(repository.NewMasterIndex())
	if err := repo.LoadIndex(); err != nil {
		t.Fatalf("error loading new index: %v", err)
	}
}

func TestRepack(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	createRandomBlobs(t, repo, rand.Intn(400), 0.7)

	packsBefore := listPacks(t, repo)

	// Running repack on empty ID sets should not do anything at all.
	repack(t, repo, nil, nil)

	packsAfter := listPacks(t, repo)

	if !packsAfter.Equals(packsBefore) {
		t.Fatalf("packs are not equal, Repack modified something. Before:\n  %v\nAfter:\n  %v",
			packsBefore, packsAfter)
	}

	saveIndex(t, repo)

	removeBlobs, keepBlobs := selectBlobs(t, repo, 0.2)

	removePacks := findPacksForBlobs(t, repo, removeBlobs)

	repack(t, repo, removePacks, keepBlobs)
	rebuildIndex(t, repo)
	reloadIndex(t, repo)

	packsAfter = listPacks(t, repo)
	for id := range removePacks {
		if packsAfter.Has(id) {
			t.Errorf("pack %v still present although it should have been repacked and removed", id.Str())
		}
	}

	idx := repo.Index()
	for id := range keepBlobs {
		pb, err := idx.Lookup(id)
		if err != nil {
			t.Errorf("unable to find blob %v in repo", id.Str())
		}

		if removePacks.Has(pb.PackID) {
			t.Errorf("lookup returned pack ID %v that should've been removed", pb.PackID)
		}
	}

	for id := range removeBlobs {
		if _, err := idx.Lookup(id); err == nil {
			t.Errorf("blob %v still contained in the repo", id.Str())
		}
	}
}
