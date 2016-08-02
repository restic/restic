package restic

import (
	"encoding/json"
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

type fakeFileSystem struct {
	t          testing.TB
	repo       *repository.Repository
	knownBlobs backend.IDSet
}

// saveFile reads from rd and saves the blobs in the repository. The list of
// IDs is returned.
func (fs fakeFileSystem) saveFile(rd io.Reader) (blobs backend.IDs) {
	blobs = backend.IDs{}
	ch := chunker.New(rd, fs.repo.Config.ChunkerPolynomial)

	for {
		chunk, err := ch.Next(getBuf())
		if err == io.EOF {
			break
		}

		if err != nil {
			fs.t.Fatalf("unable to save chunk in repo: %v", err)
		}

		id := backend.Hash(chunk.Data)
		if !fs.blobIsKnown(id) {
			_, err := fs.repo.SaveAndEncrypt(pack.Data, chunk.Data, &id)
			if err != nil {
				fs.t.Fatalf("error saving chunk: %v", err)
			}

			fs.knownBlobs.Insert(id)
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

func (fs fakeFileSystem) treeIsKnown(tree *Tree) (bool, backend.ID) {
	data, err := json.Marshal(tree)
	if err != nil {
		fs.t.Fatalf("json.Marshal(tree) returned error: %v", err)
		return false, backend.ID{}
	}
	data = append(data, '\n')

	id := backend.Hash(data)
	return fs.blobIsKnown(id), id

}

func (fs fakeFileSystem) blobIsKnown(id backend.ID) bool {
	if fs.knownBlobs.Has(id) {
		return true
	}

	if fs.repo.Index().Has(id) {
		return true
	}

	fs.knownBlobs.Insert(id)
	return false
}

// saveTree saves a tree of fake files in the repo and returns the ID.
func (fs fakeFileSystem) saveTree(seed int64, depth int) backend.ID {
	rnd := rand.NewSource(seed)
	numNodes := int(rnd.Int63() % maxNodes)

	var tree Tree
	for i := 0; i < numNodes; i++ {

		// randomly select the type of the node, either tree (p = 1/4) or file (p = 3/4).
		if depth > 1 && rnd.Int63()%4 == 0 {
			treeSeed := rnd.Int63() % maxSeed
			id := fs.saveTree(treeSeed, depth-1)

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

		node.Content = fs.saveFile(fakeFile(fs.t, fileSeed, fileSize))
		tree.Nodes = append(tree.Nodes, node)
	}

	if known, id := fs.treeIsKnown(&tree); known {
		return id
	}

	id, err := fs.repo.SaveJSON(pack.Tree, tree)
	if err != nil {
		fs.t.Fatal(err)
	}

	return id
}

// TestCreateSnapshot creates a snapshot filled with fake data. The
// fake data is generated deterministically from the timestamp `at`, which is
// also used as the snapshot's timestamp. The tree's depth can be specified
// with the parameter depth.
func TestCreateSnapshot(t testing.TB, repo *repository.Repository, at time.Time, depth int) *Snapshot {
	seed := at.Unix()
	t.Logf("create fake snapshot at %s with seed %d", at, seed)

	fakedir := fmt.Sprintf("fakedir-at-%v", at.Format("2006-01-02 15:04:05"))
	snapshot, err := NewSnapshot([]string{fakedir})
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Time = at

	fs := fakeFileSystem{
		t:          t,
		repo:       repo,
		knownBlobs: backend.NewIDSet(),
	}

	treeID := fs.saveTree(seed, depth)
	snapshot.Tree = &treeID

	id, err := repo.SaveJSONUnpacked(backend.Snapshot, snapshot)
	if err != nil {
		t.Fatal(err)
	}

	snapshot.id = &id

	t.Logf("saved snapshot %v", id.Str())

	err = repo.Flush()
	if err != nil {
		t.Fatal(err)
	}

	err = repo.SaveIndex()
	if err != nil {
		t.Fatal(err)
	}

	return snapshot
}
