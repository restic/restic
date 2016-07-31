package restic

import (
	"fmt"
	"io"
	"math/rand"
	"restic/backend"
	"restic/pack"
	"restic/repository"
	"testing"
	"time"

	"github.com/restic/chunker"
)

// fakeFile returns a reader which yields deterministic pseudo-random data.
func fakeFile(t testing.TB, seed, size int64) io.Reader {
	return io.LimitReader(repository.NewRandReader(rand.New(rand.NewSource(seed))), size)
}

// saveFile reads from rd and saves the blobs in the repository. The list of
// IDs is returned.
func saveFile(t testing.TB, repo *repository.Repository, rd io.Reader) (blobs backend.IDs) {
	ch := chunker.New(rd, repo.Config.ChunkerPolynomial)

	for {
		chunk, err := ch.Next(getBuf())
		if err == io.EOF {
			break
		}

		if err != nil {
			t.Fatalf("unabel to save chunk in repo: %v", err)
		}

		id, err := repo.SaveAndEncrypt(pack.Data, chunk.Data, nil)
		if err != nil {
			t.Fatalf("error saving chunk: %v", err)
		}
		blobs = append(blobs, id)
	}

	return blobs
}

const maxFileSize = 1500000
const maxSeed = 100

// saveTree saves a tree of fake files in the repo and returns the ID.
func saveTree(t testing.TB, repo *repository.Repository, seed int64) backend.ID {
	rnd := rand.NewSource(seed)
	numNodes := int(rnd.Int63() % 64)
	t.Logf("create %v nodes", numNodes)

	var tree Tree
	for i := 0; i < numNodes; i++ {
		seed := rnd.Int63() % maxSeed
		size := rnd.Int63() % maxFileSize

		node := &Node{
			Name: fmt.Sprintf("file-%v", seed),
			Type: "file",
			Mode: 0644,
			Size: uint64(size),
		}

		node.Content = saveFile(t, repo, fakeFile(t, seed, size))
		tree.Nodes = append(tree.Nodes, node)
	}

	id, err := repo.SaveJSON(pack.Tree, tree)
	if err != nil {
		t.Fatal(err)
	}

	return id
}

// TestCreateSnapshot creates a snapshot filled with fake data. The
// fake data is generated deterministically from the timestamp `at`, which is
// also used as the snapshot's timestamp.
func TestCreateSnapshot(t testing.TB, repo *repository.Repository, at time.Time) backend.ID {
	fakedir := fmt.Sprintf("fakedir-at-%v", at.Format("2006-01-02 15:04:05"))
	snapshot, err := NewSnapshot([]string{fakedir})
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Time = at

	treeID := saveTree(t, repo, at.UnixNano())
	snapshot.Tree = &treeID

	id, err := repo.SaveJSONUnpacked(backend.Snapshot, snapshot)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("saved snapshot %v", id.Str())

	err = repo.Flush()
	if err != nil {
		t.Fatal(err)
	}

	err = repo.SaveIndex()
	if err != nil {
		t.Fatal(err)
	}

	return id
}
