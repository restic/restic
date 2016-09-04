package repository_test

import (
	"io"
	"math/rand"
	"restic"
	"restic/repository"
	"testing"
)

func randomSize(min, max int) int {
	return rand.Intn(max-min) + min
}

func random(t testing.TB, length int) []byte {
	rd := restic.NewRandReader(rand.New(rand.NewSource(int64(length))))
	buf := make([]byte, length)
	_, err := io.ReadFull(rd, buf)
	if err != nil {
		t.Fatalf("unable to read %d random bytes: %v", length, err)
	}

	return buf
}

func createRandomBlobs(t testing.TB, repo restic.Repository, blobs int, pData float32) {
	for i := 0; i < blobs; i++ {
		var (
			tpe    restic.BlobType
			length int
		)

		if rand.Float32() < pData {
			tpe = restic.DataBlob
			length = randomSize(10*1024, 1024*1024) // 10KiB to 1MiB of data
		} else {
			tpe = restic.TreeBlob
			length = randomSize(1*1024, 20*1024) // 1KiB to 20KiB
		}

		buf := random(t, length)
		id := restic.Hash(buf)

		if repo.Index().Has(id, restic.DataBlob) {
			t.Errorf("duplicate blob %v/%v ignored", id, restic.DataBlob)
			continue
		}

		_, err := repo.SaveBlob(tpe, buf, id)
		if err != nil {
			t.Fatalf("SaveFrom() error %v", err)
		}

		if rand.Float32() < 0.2 {
			if err = repo.Flush(); err != nil {
				t.Fatalf("repo.Flush() returned error %v", err)
			}
		}
	}

	if err := repo.Flush(); err != nil {
		t.Fatalf("repo.Flush() returned error %v", err)
	}
}

// selectBlobs splits the list of all blobs randomly into two lists. A blob
// will be contained in the firstone ith probability p.
func selectBlobs(t *testing.T, repo restic.Repository, p float32) (list1, list2 restic.BlobSet) {
	done := make(chan struct{})
	defer close(done)

	list1 = restic.NewBlobSet()
	list2 = restic.NewBlobSet()

	blobs := restic.NewBlobSet()

	for id := range repo.List(restic.DataFile, done) {
		entries, _, err := repo.ListPack(id)
		if err != nil {
			t.Fatalf("error listing pack %v: %v", id, err)
		}

		for _, entry := range entries {
			h := restic.BlobHandle{ID: entry.ID, Type: entry.Type}
			if blobs.Has(h) {
				t.Errorf("ignoring duplicate blob %v", h)
				continue
			}
			blobs.Insert(h)

			if rand.Float32() <= p {
				list1.Insert(restic.BlobHandle{ID: entry.ID, Type: entry.Type})
			} else {
				list2.Insert(restic.BlobHandle{ID: entry.ID, Type: entry.Type})
			}

		}
	}

	return list1, list2
}

func listPacks(t *testing.T, repo restic.Repository) restic.IDSet {
	done := make(chan struct{})
	defer close(done)

	list := restic.NewIDSet()
	for id := range repo.List(restic.DataFile, done) {
		list.Insert(id)
	}

	return list
}

func findPacksForBlobs(t *testing.T, repo restic.Repository, blobs restic.BlobSet) restic.IDSet {
	packs := restic.NewIDSet()

	idx := repo.Index()
	for h := range blobs {
		list, err := idx.Lookup(h.ID, h.Type)
		if err != nil {
			t.Fatal(err)
		}

		for _, pb := range list {
			packs.Insert(pb.PackID)
		}
	}

	return packs
}

func repack(t *testing.T, repo restic.Repository, packs restic.IDSet, blobs restic.BlobSet) {
	err := repository.Repack(repo, packs, blobs)
	if err != nil {
		t.Fatal(err)
	}
}

func saveIndex(t *testing.T, repo restic.Repository) {
	if err := repo.SaveIndex(); err != nil {
		t.Fatalf("repo.SaveIndex() %v", err)
	}
}

func rebuildIndex(t *testing.T, repo restic.Repository) {
	if err := repository.RebuildIndex(repo); err != nil {
		t.Fatalf("error rebuilding index: %v", err)
	}
}

func reloadIndex(t *testing.T, repo restic.Repository) {
	repo.SetIndex(repository.NewMasterIndex())
	if err := repo.LoadIndex(); err != nil {
		t.Fatalf("error loading new index: %v", err)
	}
}

func TestRepack(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	createRandomBlobs(t, repo, 100, 0.7)

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

	for h := range keepBlobs {
		list, err := idx.Lookup(h.ID, h.Type)
		if err != nil {
			t.Errorf("unable to find blob %v in repo", h.ID.Str())
			continue
		}

		if len(list) != 1 {
			t.Errorf("expected one pack in the list, got: %v", list)
			continue
		}

		pb := list[0]

		if removePacks.Has(pb.PackID) {
			t.Errorf("lookup returned pack ID %v that should've been removed", pb.PackID)
		}
	}

	for h := range removeBlobs {
		if _, err := idx.Lookup(h.ID, h.Type); err == nil {
			t.Errorf("blob %v still contained in the repo", h)
		}
	}
}
