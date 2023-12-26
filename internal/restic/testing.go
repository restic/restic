package restic

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"testing"
	"time"

	"github.com/restic/chunker"
	"golang.org/x/sync/errgroup"
)

// fakeFile returns a reader which yields deterministic pseudo-random data.
func fakeFile(seed, size int64) io.Reader {
	return io.LimitReader(rand.New(rand.NewSource(seed)), size)
}

type fakeFileSystem struct {
	t       testing.TB
	repo    Repository
	buf     []byte
	chunker *chunker.Chunker
	rand    *rand.Rand
}

// saveFile reads from rd and saves the blobs in the repository. The list of
// IDs is returned.
func (fs *fakeFileSystem) saveFile(ctx context.Context, rd io.Reader) (blobs IDs) {
	if fs.buf == nil {
		fs.buf = make([]byte, chunker.MaxSize)
	}

	if fs.chunker == nil {
		fs.chunker = chunker.New(rd, fs.repo.Config().ChunkerPolynomial)
	} else {
		fs.chunker.Reset(rd, fs.repo.Config().ChunkerPolynomial)
	}

	blobs = IDs{}
	for {
		chunk, err := fs.chunker.Next(fs.buf)
		if err == io.EOF {
			break
		}

		if err != nil {
			fs.t.Fatalf("unable to save chunk in repo: %v", err)
		}

		id, _, _, err := fs.repo.SaveBlob(ctx, DataBlob, chunk.Data, ID{}, false)
		if err != nil {
			fs.t.Fatalf("error saving chunk: %v", err)
		}

		blobs = append(blobs, id)
	}

	return blobs
}

const (
	maxFileSize = 20000
	maxSeed     = 32
	maxNodes    = 15
)

// saveTree saves a tree of fake files in the repo and returns the ID.
func (fs *fakeFileSystem) saveTree(ctx context.Context, seed int64, depth int) ID {
	rnd := rand.NewSource(seed)
	numNodes := int(rnd.Int63() % maxNodes)

	var tree Tree
	for i := 0; i < numNodes; i++ {

		// randomly select the type of the node, either tree (p = 1/4) or file (p = 3/4).
		if depth > 1 && rnd.Int63()%4 == 0 {
			treeSeed := rnd.Int63() % maxSeed
			id := fs.saveTree(ctx, treeSeed, depth-1)

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

		node.Content = fs.saveFile(ctx, fakeFile(fileSeed, fileSize))
		tree.Nodes = append(tree.Nodes, node)
	}

	tree.Sort()

	id, err := SaveTree(ctx, fs.repo, &tree)
	if err != nil {
		fs.t.Fatalf("SaveTree returned error: %v", err)
	}
	return id
}

// TestCreateSnapshot creates a snapshot filled with fake data. The
// fake data is generated deterministically from the timestamp `at`, which is
// also used as the snapshot's timestamp. The tree's depth can be specified
// with the parameter depth. The parameter duplication is a probability that
// the same blob will saved again.
func TestCreateSnapshot(t testing.TB, repo Repository, at time.Time, depth int) *Snapshot {
	seed := at.Unix()
	t.Logf("create fake snapshot at %s with seed %d", at, seed)

	fakedir := fmt.Sprintf("fakedir-at-%v", at.Format("2006-01-02 15:04:05"))
	snapshot, err := NewSnapshot([]string{fakedir}, []string{"test"}, "foo", at)
	if err != nil {
		t.Fatal(err)
	}

	fs := fakeFileSystem{
		t:    t,
		repo: repo,
		rand: rand.New(rand.NewSource(seed)),
	}

	var wg errgroup.Group
	repo.StartPackUploader(context.TODO(), &wg)

	treeID := fs.saveTree(context.TODO(), seed, depth)
	snapshot.Tree = &treeID

	err = repo.Flush(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	id, err := SaveSnapshot(context.TODO(), repo, snapshot)
	if err != nil {
		t.Fatal(err)
	}

	snapshot.id = &id

	t.Logf("saved snapshot %v", id.Str())

	return snapshot
}

// TestParseID parses s as a ID and panics if that fails.
func TestParseID(s string) ID {
	id, err := ParseID(s)
	if err != nil {
		panic(fmt.Sprintf("unable to parse string %q as ID: %v", s, err))
	}

	return id
}

// TestParseHandle parses s as a ID, panics if that fails and creates a BlobHandle with t.
func TestParseHandle(s string, t BlobType) BlobHandle {
	return BlobHandle{ID: TestParseID(s), Type: t}
}

// TestSetSnapshotID sets the snapshot's ID.
func TestSetSnapshotID(_ testing.TB, sn *Snapshot, id ID) {
	sn.id = &id
}

// ParseDurationOrPanic parses a duration from a string or panics if string is invalid.
// The format is `6y5m234d37h`.
func ParseDurationOrPanic(s string) Duration {
	d, err := ParseDuration(s)
	if err != nil {
		panic(err)
	}

	return d
}

// TestLoadAllSnapshots returns a list of all snapshots in the repo.
// If a snapshot ID is in excludeIDs, it will not be included in the result.
func TestLoadAllSnapshots(ctx context.Context, repo Repository, excludeIDs IDSet) (snapshots Snapshots, err error) {
	err = ForAllSnapshots(ctx, repo, repo, excludeIDs, func(id ID, sn *Snapshot, err error) error {
		if err != nil {
			return err
		}

		snapshots = append(snapshots, sn)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return snapshots, nil
}
