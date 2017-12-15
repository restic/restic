package archiver_test

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/restic/restic/internal/archiver"
	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"

	"github.com/restic/restic/internal/errors"

	"github.com/restic/chunker"
)

var testPol = chunker.Pol(0x3DA3358B4DC173)

type Rdr interface {
	io.ReadSeeker
	io.ReaderAt
}

func benchmarkChunkEncrypt(b testing.TB, buf, buf2 []byte, rd Rdr, key *crypto.Key) {
	rd.Seek(0, 0)
	ch := chunker.New(rd, testPol)
	nonce := crypto.NewRandomNonce()

	for {
		chunk, err := ch.Next(buf)

		if errors.Cause(err) == io.EOF {
			break
		}

		rtest.OK(b, err)

		rtest.Assert(b, uint(len(chunk.Data)) == chunk.Length,
			"invalid length: got %d, expected %d", len(chunk.Data), chunk.Length)

		_ = key.Seal(buf2[:0], nonce, chunk.Data, nil)
	}
}

func BenchmarkChunkEncrypt(b *testing.B) {
	repo, cleanup := repository.TestRepository(b)
	defer cleanup()

	data := rtest.Random(23, 10<<20) // 10MiB
	rd := bytes.NewReader(data)

	buf := make([]byte, chunker.MaxSize)
	buf2 := make([]byte, chunker.MaxSize)

	b.ResetTimer()
	b.SetBytes(int64(len(data)))

	for i := 0; i < b.N; i++ {
		benchmarkChunkEncrypt(b, buf, buf2, rd, repo.Key())
	}
}

func benchmarkChunkEncryptP(b *testing.PB, buf []byte, rd Rdr, key *crypto.Key) {
	ch := chunker.New(rd, testPol)
	nonce := crypto.NewRandomNonce()

	for {
		chunk, err := ch.Next(buf)
		if errors.Cause(err) == io.EOF {
			break
		}

		_ = key.Seal(chunk.Data[:0], nonce, chunk.Data, nil)
	}
}

func BenchmarkChunkEncryptParallel(b *testing.B) {
	repo, cleanup := repository.TestRepository(b)
	defer cleanup()

	data := rtest.Random(23, 10<<20) // 10MiB

	buf := make([]byte, chunker.MaxSize)

	b.ResetTimer()
	b.SetBytes(int64(len(data)))

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rd := bytes.NewReader(data)
			benchmarkChunkEncryptP(pb, buf, rd, repo.Key())
		}
	})
}

func archiveDirectory(b testing.TB) {
	repo, cleanup := repository.TestRepository(b)
	defer cleanup()

	arch := archiver.New(repo)

	_, id, err := arch.Snapshot(context.TODO(), nil, []string{rtest.BenchArchiveDirectory}, nil, "localhost", nil, time.Now())
	rtest.OK(b, err)

	b.Logf("snapshot archived as %v", id)
}

func TestArchiveDirectory(t *testing.T) {
	if rtest.BenchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping TestArchiveDirectory")
	}

	archiveDirectory(t)
}

func BenchmarkArchiveDirectory(b *testing.B) {
	if rtest.BenchArchiveDirectory == "" {
		b.Skip("benchdir not set, skipping BenchmarkArchiveDirectory")
	}

	for i := 0; i < b.N; i++ {
		archiveDirectory(b)
	}
}

func countPacks(t testing.TB, repo restic.Repository, tpe restic.FileType) (n uint) {
	err := repo.Backend().List(context.TODO(), tpe, func(restic.FileInfo) error {
		n++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	return n
}

func archiveWithDedup(t testing.TB) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	if rtest.BenchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping TestArchiverDedup")
	}

	var cnt struct {
		before, after, after2 struct {
			packs, dataBlobs, treeBlobs uint
		}
	}

	// archive a few files
	sn := archiver.TestSnapshot(t, repo, rtest.BenchArchiveDirectory, nil)
	t.Logf("archived snapshot %v", sn.ID().Str())

	// get archive stats
	cnt.before.packs = countPacks(t, repo, restic.DataFile)
	cnt.before.dataBlobs = repo.Index().Count(restic.DataBlob)
	cnt.before.treeBlobs = repo.Index().Count(restic.TreeBlob)
	t.Logf("packs %v, data blobs %v, tree blobs %v",
		cnt.before.packs, cnt.before.dataBlobs, cnt.before.treeBlobs)

	// archive the same files again, without parent snapshot
	sn2 := archiver.TestSnapshot(t, repo, rtest.BenchArchiveDirectory, nil)
	t.Logf("archived snapshot %v", sn2.ID().Str())

	// get archive stats again
	cnt.after.packs = countPacks(t, repo, restic.DataFile)
	cnt.after.dataBlobs = repo.Index().Count(restic.DataBlob)
	cnt.after.treeBlobs = repo.Index().Count(restic.TreeBlob)
	t.Logf("packs %v, data blobs %v, tree blobs %v",
		cnt.after.packs, cnt.after.dataBlobs, cnt.after.treeBlobs)

	// if there are more data blobs, something is wrong
	if cnt.after.dataBlobs > cnt.before.dataBlobs {
		t.Fatalf("TestArchiverDedup: too many data blobs in repository: before %d, after %d",
			cnt.before.dataBlobs, cnt.after.dataBlobs)
	}

	// archive the same files again, with a parent snapshot
	sn3 := archiver.TestSnapshot(t, repo, rtest.BenchArchiveDirectory, sn2.ID())
	t.Logf("archived snapshot %v, parent %v", sn3.ID().Str(), sn2.ID().Str())

	// get archive stats again
	cnt.after2.packs = countPacks(t, repo, restic.DataFile)
	cnt.after2.dataBlobs = repo.Index().Count(restic.DataBlob)
	cnt.after2.treeBlobs = repo.Index().Count(restic.TreeBlob)
	t.Logf("packs %v, data blobs %v, tree blobs %v",
		cnt.after2.packs, cnt.after2.dataBlobs, cnt.after2.treeBlobs)

	// if there are more data blobs, something is wrong
	if cnt.after2.dataBlobs > cnt.before.dataBlobs {
		t.Fatalf("TestArchiverDedup: too many data blobs in repository: before %d, after %d",
			cnt.before.dataBlobs, cnt.after2.dataBlobs)
	}
}

func TestArchiveDedup(t *testing.T) {
	archiveWithDedup(t)
}

func TestArchiveEmptySnapshot(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	arch := archiver.New(repo)

	sn, id, err := arch.Snapshot(context.TODO(), nil, []string{"file-does-not-exist-123123213123", "file2-does-not-exist-too-123123123"}, nil, "localhost", nil, time.Now())
	if err == nil {
		t.Errorf("expected error for empty snapshot, got nil")
	}

	if !id.IsNull() {
		t.Errorf("expected null ID for empty snapshot, got %v", id.Str())
	}

	if sn != nil {
		t.Errorf("expected null snapshot for empty snapshot, got %v", sn)
	}
}

func TestArchiveNameCollision(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	dir, cleanup := rtest.TempDir(t)
	defer cleanup()

	root := filepath.Join(dir, "root")
	rtest.OK(t, os.MkdirAll(root, 0755))

	rtest.OK(t, ioutil.WriteFile(filepath.Join(dir, "testfile"), []byte("testfile1"), 0644))
	rtest.OK(t, ioutil.WriteFile(filepath.Join(dir, "root", "testfile"), []byte("testfile2"), 0644))

	defer fs.TestChdir(t, root)()

	arch := archiver.New(repo)

	sn, id, err := arch.Snapshot(context.TODO(), nil, []string{"testfile", filepath.Join("..", "testfile")}, nil, "localhost", nil, time.Now())
	rtest.OK(t, err)

	t.Logf("snapshot archived as %v", id)

	tree, err := repo.LoadTree(context.TODO(), *sn.Tree)
	rtest.OK(t, err)

	if len(tree.Nodes) != 2 {
		t.Fatalf("tree has %d nodes, wanted 2: %v", len(tree.Nodes), tree.Nodes)
	}
}
