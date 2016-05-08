package restic

import (
	"bytes"
	"io"
	"math/rand"
	"restic/backend"
	"restic/pack"
	"restic/repository"
	"testing"

	"github.com/restic/chunker"
)

func loadBlob(t *testing.T, repo *repository.Repository, id backend.ID, buf []byte) []byte {
	buf, err := repo.LoadBlob(pack.Data, id, buf)
	if err != nil {
		t.Fatalf("LoadBlob(%v) returned error %v", id, err)
	}

	return buf
}

func TestArchiveReader(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	seed := rand.Int63()
	size := int64(rand.Intn(50*1024*1024) + 50*1024*1024)
	t.Logf("seed is 0x%016x, size is %v", seed, size)

	f := fakeFile(t, seed, size)

	sn, id, err := ArchiveReader(repo, nil, f, "fakefile")
	if err != nil {
		t.Fatalf("ArchiveReader() returned error %v", err)
	}

	if id.IsNull() {
		t.Fatalf("ArchiveReader() returned null ID")
	}

	t.Logf("snapshot saved as %v, tree is %v", id.Str(), sn.Tree.Str())

	tree, err := LoadTree(repo, *sn.Tree)
	if err != nil {
		t.Fatalf("LoadTree() returned error %v", err)
	}

	if len(tree.Nodes) != 1 {
		t.Fatalf("wrong number of nodes for tree, want %v, got %v", 1, len(tree.Nodes))
	}

	node := tree.Nodes[0]
	if node.Name != "fakefile" {
		t.Fatalf("wrong filename, want %v, got %v", "fakefile", node.Name)
	}

	if len(node.Content) == 0 {
		t.Fatalf("node.Content has length 0")
	}

	// check blobs
	f = fakeFile(t, seed, size)
	buf := make([]byte, chunker.MaxSize)
	buf2 := make([]byte, chunker.MaxSize)
	for i, id := range node.Content {
		buf = loadBlob(t, repo, id, buf)

		buf2 = buf2[:len(buf)]
		_, err = io.ReadFull(f, buf2)

		if !bytes.Equal(buf, buf2) {
			t.Fatalf("blob %d (%v) is wrong", i, id.Str())
		}
	}
}
