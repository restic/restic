package repository_test

import (
	"io"
	"math/rand"
	"restic"
	"restic/backend"
	"restic/pack"
	"restic/repository"
	"testing"
	"time"
)

func randomSize(min, max int) int {
	return rand.Intn(max-min) + min
}

func random(t *testing.T, length int) []byte {
	rd := repository.NewRandReader(rand.New(rand.NewSource(int64(length))))
	buf := make([]byte, length)
	_, err := io.ReadFull(rd, buf)
	if err != nil {
		t.Fatalf("unable to read %d random bytes: %v", length, err)
	}

	return buf
}

func createRandomBlobs(t *testing.T, repo *repository.Repository, blobs int, pData float32) {
	for i := 0; i < blobs; i++ {
		var (
			tpe    pack.BlobType
			length int
		)

		if rand.Float32() < pData {
			tpe = pack.Data
			length = randomSize(50*1024, 2*1024*1024) // 50KiB to 2MiB of data
		} else {
			tpe = pack.Tree
			length = randomSize(5*1024, 50*1024) // 5KiB to 50KiB
		}

		_, err := repo.SaveAndEncrypt(tpe, random(t, length), nil)
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
func selectBlobs(t *testing.T, repo *repository.Repository, p float32) (list1, list2 backend.IDSet) {
	done := make(chan struct{})
	defer close(done)

	list1 = backend.NewIDSet()
	list2 = backend.NewIDSet()

	for id := range repo.List(backend.Data, done) {
		entries, err := repo.ListPack(id)
		if err != nil {
			t.Fatalf("error listing pack %v: %v", id, err)
		}

		for _, entry := range entries {
			if rand.Float32() <= p {
				list1.Insert(entry.ID)
			} else {
				list2.Insert(entry.ID)
			}
		}
	}

	return list1, list2
}

func listPacks(t *testing.T, repo *repository.Repository) backend.IDSet {
	done := make(chan struct{})
	defer close(done)

	list := backend.NewIDSet()
	for id := range repo.List(backend.Data, done) {
		list.Insert(id)
	}

	return list
}

func findPacksForBlobs(t *testing.T, repo *repository.Repository, blobs backend.IDSet) backend.IDSet {
	packs := backend.NewIDSet()

	idx := repo.Index()
	for id := range blobs {
		pb, err := idx.Lookup(id)
		if err != nil {
			t.Fatal(err)
		}

		packs.Insert(pb.PackID)
	}

	return packs
}

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

const (
	testSnapshots = 3
	testDepth     = 2
)

var testTime = time.Unix(1469960361, 23)

func TestFindUsedBlobs(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	var snapshots []*restic.Snapshot
	for i := 0; i < testSnapshots; i++ {
		sn := restic.TestCreateSnapshot(t, repo, testTime.Add(time.Duration(i)*time.Second), testDepth)
		t.Logf("snapshot %v saved, tree %v", sn.ID().Str(), sn.Tree.Str())
		snapshots = append(snapshots, sn)
	}

	for _, sn := range snapshots {
		usedBlobs, err := repository.FindUsedBlobs(repo, *sn.Tree)
		if err != nil {
			t.Errorf("FindUsedBlobs returned error: %v", err)
			continue
		}

		if len(usedBlobs) == 0 {
			t.Errorf("FindUsedBlobs returned an empty set")
			continue
		}

		t.Logf("used blobs from snapshot %v (tree %v): %v", sn.ID().Str(), sn.Tree.Str(), usedBlobs)
	}
}
