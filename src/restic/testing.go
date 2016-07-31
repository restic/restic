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
	blobs = backend.IDs{}
	ch := chunker.New(rd, repo.Config.ChunkerPolynomial)

	for {
		chunk, err := ch.Next(getBuf())
		if err == io.EOF {
			break
		}

		if err != nil {
			t.Fatalf("unable to save chunk in repo: %v", err)
		}

		id, err := repo.SaveAndEncrypt(pack.Data, chunk.Data, nil)
		if err != nil {
			t.Fatalf("error saving chunk: %v", err)
		}
		blobs = append(blobs, id)
	}

	return blobs
}

const (
	maxFileSize = 1500000
	maxSeed     = 32
	maxNodes    = 32
)

// saveTree saves a tree of fake files in the repo and returns the ID.
func saveTree(t testing.TB, repo *repository.Repository, seed int64, depth int) backend.ID {
	rnd := rand.NewSource(seed)
	numNodes := int(rnd.Int63() % maxNodes)

	var tree Tree
	for i := 0; i < numNodes; i++ {

		// randomly select the type of the node, either tree (p = 1/4) or file (p = 3/4).
		if depth > 1 && rnd.Int63()%4 == 0 {
			treeSeed := rnd.Int63() % maxSeed
			id := saveTree(t, repo, treeSeed, depth-1)

			node := &Node{
				Name:    fmt.Sprintf("dir-%v", treeSeed),
				Type:    "dir",
				Mode:    0755,
				Subtree: &id,
			}

			tree.Nodes = append(tree.Nodes, node)
			continue
		}

		fileSeed := rnd.Int63() % maxSeed
		fileSize := (maxFileSize / maxSeed) * fileSeed

		node := &Node{
			Name: fmt.Sprintf("file-%v", fileSeed),
			Type: "file",
			Mode: 0644,
			Size: uint64(fileSize),
		}

		node.Content = saveFile(t, repo, fakeFile(t, fileSeed, fileSize))
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
// also used as the snapshot's timestamp. The tree's depth can be specified
// with the parameter depth.
func TestCreateSnapshot(t testing.TB, repo *repository.Repository, at time.Time, depth int) backend.ID {
	seed := at.Unix()
	t.Logf("create fake snapshot at %s with seed %d", at, seed)

	fakedir := fmt.Sprintf("fakedir-at-%v", at.Format("2006-01-02 15:04:05"))
	snapshot, err := NewSnapshot([]string{fakedir})
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Time = at

	treeID := saveTree(t, repo, seed, depth)
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
