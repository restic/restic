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

type randReader struct {
	rnd *rand.Rand
	buf []byte
}

func newRandReader(rnd *rand.Rand) io.Reader {
	return &randReader{rnd: rnd, buf: make([]byte, 0, 7)}
}

func (rd *randReader) read(p []byte) (n int, err error) {
	if len(p)%7 != 0 {
		panic("invalid buffer length, not multiple of 7")
	}

	rnd := rd.rnd
	for i := 0; i < len(p); i += 7 {
		val := rnd.Int63()

		p[i+0] = byte(val >> 0)
		p[i+1] = byte(val >> 8)
		p[i+2] = byte(val >> 16)
		p[i+3] = byte(val >> 24)
		p[i+4] = byte(val >> 32)
		p[i+5] = byte(val >> 40)
		p[i+6] = byte(val >> 48)
	}

	return len(p), nil
}

func (rd *randReader) Read(p []byte) (int, error) {
	// first, copy buffer to p
	pos := copy(p, rd.buf)
	copy(rd.buf, rd.buf[pos:])

	// shorten buf and p accordingly
	rd.buf = rd.buf[:len(rd.buf)-pos]
	p = p[pos:]

	// if this is enough to fill p, return
	if len(p) == 0 {
		return pos, nil
	}

	// load multiple of 7 byte
	l := (len(p) / 7) * 7
	n, err := rd.read(p[:l])
	pos += n
	if err != nil {
		return pos, err
	}
	p = p[n:]

	// load 7 byte to temp buffer
	rd.buf = rd.buf[:7]
	n, err = rd.read(rd.buf)
	if err != nil {
		return pos, err
	}

	// copy the remaining bytes from the buffer to p
	n = copy(p, rd.buf)
	pos += n

	// save the remaining bytes in rd.buf
	n = copy(rd.buf, rd.buf[n:])
	rd.buf = rd.buf[:n]

	return pos, nil
}

// fakeFile returns a reader which yields deterministic pseudo-random data.
func fakeFile(t testing.TB, seed, size int64) io.Reader {
	return io.LimitReader(newRandReader(rand.New(rand.NewSource(seed))), size)
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

// TestRepositoryCreateSnapshot creates a snapshot filled with fake data. The
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
