package repository

import (
	"io"
	"math/rand"
	"restic/backend"
	"restic/pack"
	"testing"
)

func randomSize(min, max int) int {
	return rand.Intn(max-min) + min
}

func random(t *testing.T, length int) []byte {
	src := rand.New(rand.NewSource(int64(length)))
	buf := make([]byte, length)
	_, err := io.ReadFull(src, buf)
	if err != nil {
		t.Fatalf("unable to read %d random bytes: %v", length, err)
	}

	return buf
}

func createRandomBlobs(t *testing.T, repo *Repository, blobs int, pData float32) {
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

// redundancy returns the amount of duplicate data in the repo. It only looks
// at all pack files.
func redundancy(t *testing.T, repo *Repository) float32 {
	done := make(chan struct{})
	defer close(done)

	type redEntry struct {
		count int
		size  int
	}
	red := make(map[backend.ID]redEntry)

	for id := range repo.List(backend.Data, done) {
		entries, err := repo.ListPack(id)
		if err != nil {
			t.Fatalf("error listing pack %v: %v", id.Str(), err)
		}

		for _, e := range entries {
			updatedEntry := redEntry{
				count: 1,
				size:  int(e.Length),
			}

			if oldEntry, ok := red[e.ID]; ok {
				updatedEntry.count += oldEntry.count

				if updatedEntry.size != oldEntry.size {
					t.Fatalf("sizes do not match: %v != %v", updatedEntry.size, oldEntry.size)
				}
			}

			red[e.ID] = updatedEntry
		}
	}

	totalBytes := 0
	redundantBytes := 0
	for _, v := range red {
		totalBytes += v.count * v.size

		if v.count > 1 {
			redundantBytes += (v.count - 1) * v.size
		}
	}

	return float32(redundantBytes) / float32(totalBytes)
}

// selectBlobs returns a list of random blobs from the repository with probability p.
func selectBlobs(t *testing.T, repo *Repository, p float32) backend.IDSet {
	done := make(chan struct{})
	defer close(done)

	blobs := backend.NewIDSet()

	for id := range repo.List(backend.Data, done) {
		entries, err := repo.ListPack(id)
		if err != nil {
			t.Fatalf("error listing pack %v: %v", id, err)
		}

		for _, entry := range entries {
			if rand.Float32() <= p {
				blobs.Insert(entry.ID)
			}
		}
	}

	return blobs
}

func listPacks(t *testing.T, repo *Repository) backend.IDSet {
	done := make(chan struct{})
	defer close(done)

	list := backend.NewIDSet()
	for id := range repo.List(backend.Data, done) {
		list.Insert(id)
	}

	return list
}

func findPacksForBlobs(t *testing.T, repo *Repository, blobs backend.IDSet) backend.IDSet {
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

func TestRepack(t *testing.T) {
	repo, cleanup := TestRepository(t)
	defer cleanup()

	createRandomBlobs(t, repo, rand.Intn(400), 0.7)

	packsBefore := listPacks(t, repo)

	// Running repack on empty ID sets should not do anything at all.
	err := Repack(repo, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	packsAfter := listPacks(t, repo)

	if !packsAfter.Equals(packsBefore) {
		t.Fatalf("packs are not equal, Repack modified something. Before:\n  %v\nAfter:\n  %v",
			packsBefore, packsAfter)
	}

	if err := repo.SaveIndex(); err != nil {
		t.Fatalf("repo.SaveIndex() %v", err)
	}

	blobs := selectBlobs(t, repo, 0.2)
	t.Logf("selected %d blobs: %v", len(blobs), blobs)

	packs := findPacksForBlobs(t, repo, blobs)

	err = Repack(repo, packs, blobs)
	if err != nil {
		t.Fatalf("Repack() error %v", err)
	}

	packsAfter = listPacks(t, repo)
	for id := range packs {
		if packsAfter.Has(id) {
			t.Errorf("pack %v still present although it should have been repacked and removed", id.Str())
		}
	}

	idx := repo.Index()
	for id := range blobs {
		pb, err := idx.Lookup(id)
		if err != nil {
			t.Errorf("unable to find blob %v in repo", id.Str())
		}

		if packs.Has(pb.PackID) {
			t.Errorf("lookup returned pack ID %v that should've been removed", pb.PackID)
		}
	}
}
