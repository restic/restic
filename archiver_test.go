package restic_test

import (
	"bytes"
	"crypto/sha256"
	"io"
	"testing"

	"github.com/restic/chunker"
	"github.com/restic/restic"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/crypto"
	"github.com/restic/restic/pack"
	. "github.com/restic/restic/test"
)

var testPol = chunker.Pol(0x3DA3358B4DC173)

type Rdr interface {
	io.ReadSeeker
	io.ReaderAt
}

func benchmarkChunkEncrypt(b testing.TB, buf, buf2 []byte, rd Rdr, key *crypto.Key) {
	rd.Seek(0, 0)
	ch := chunker.New(rd, testPol, sha256.New())

	for {
		chunk, err := ch.Next()

		if err == io.EOF {
			break
		}

		OK(b, err)

		// reduce length of buf
		buf = buf[:chunk.Length]
		n, err := io.ReadFull(chunk.Reader(rd), buf)
		OK(b, err)
		Assert(b, uint(n) == chunk.Length, "invalid length: got %d, expected %d", n, chunk.Length)

		_, err = crypto.Encrypt(key, buf2, buf)
		OK(b, err)
	}
}

func BenchmarkChunkEncrypt(b *testing.B) {
	repo := SetupRepo()
	defer TeardownRepo(repo)

	data := Random(23, 10<<20) // 10MiB
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
	ch := chunker.New(rd, testPol, sha256.New())

	for {
		chunk, err := ch.Next()
		if err == io.EOF {
			break
		}

		// reduce length of chunkBuf
		buf = buf[:chunk.Length]
		io.ReadFull(chunk.Reader(rd), buf)
		crypto.Encrypt(key, buf, buf)
	}
}

func BenchmarkChunkEncryptParallel(b *testing.B) {
	repo := SetupRepo()
	defer TeardownRepo(repo)

	data := Random(23, 10<<20) // 10MiB

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
	repo := SetupRepo()
	defer TeardownRepo(repo)

	arch := restic.NewArchiver(repo)

	_, id, err := arch.Snapshot(nil, []string{BenchArchiveDirectory}, nil)
	OK(b, err)

	b.Logf("snapshot archived as %v", id)
}

func TestArchiveDirectory(t *testing.T) {
	if BenchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping TestArchiveDirectory")
	}

	archiveDirectory(t)
}

func BenchmarkArchiveDirectory(b *testing.B) {
	if BenchArchiveDirectory == "" {
		b.Skip("benchdir not set, skipping BenchmarkArchiveDirectory")
	}

	for i := 0; i < b.N; i++ {
		archiveDirectory(b)
	}
}

func archiveWithDedup(t testing.TB) {
	repo := SetupRepo()
	defer TeardownRepo(repo)

	if BenchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping TestArchiverDedup")
	}

	var cnt struct {
		before, after, after2 struct {
			packs, dataBlobs, treeBlobs uint
		}
	}

	// archive a few files
	sn := SnapshotDir(t, repo, BenchArchiveDirectory, nil)
	t.Logf("archived snapshot %v", sn.ID().Str())

	// get archive stats
	cnt.before.packs = repo.Count(backend.Data)
	cnt.before.dataBlobs = repo.Index().Count(pack.Data)
	cnt.before.treeBlobs = repo.Index().Count(pack.Tree)
	t.Logf("packs %v, data blobs %v, tree blobs %v",
		cnt.before.packs, cnt.before.dataBlobs, cnt.before.treeBlobs)

	// archive the same files again, without parent snapshot
	sn2 := SnapshotDir(t, repo, BenchArchiveDirectory, nil)
	t.Logf("archived snapshot %v", sn2.ID().Str())

	// get archive stats again
	cnt.after.packs = repo.Count(backend.Data)
	cnt.after.dataBlobs = repo.Index().Count(pack.Data)
	cnt.after.treeBlobs = repo.Index().Count(pack.Tree)
	t.Logf("packs %v, data blobs %v, tree blobs %v",
		cnt.after.packs, cnt.after.dataBlobs, cnt.after.treeBlobs)

	// if there are more data blobs, something is wrong
	if cnt.after.dataBlobs > cnt.before.dataBlobs {
		t.Fatalf("TestArchiverDedup: too many data blobs in repository: before %d, after %d",
			cnt.before.dataBlobs, cnt.after.dataBlobs)
	}

	// archive the same files again, with a parent snapshot
	sn3 := SnapshotDir(t, repo, BenchArchiveDirectory, sn2.ID())
	t.Logf("archived snapshot %v, parent %v", sn3.ID().Str(), sn2.ID().Str())

	// get archive stats again
	cnt.after2.packs = repo.Count(backend.Data)
	cnt.after2.dataBlobs = repo.Index().Count(pack.Data)
	cnt.after2.treeBlobs = repo.Index().Count(pack.Tree)
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

func BenchmarkLoadTree(t *testing.B) {
	repo := SetupRepo()
	defer TeardownRepo(repo)

	if BenchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping TestArchiverDedup")
	}

	// archive a few files
	arch := restic.NewArchiver(repo)
	sn, _, err := arch.Snapshot(nil, []string{BenchArchiveDirectory}, nil)
	OK(t, err)
	t.Logf("archived snapshot %v", sn.ID())

	list := make([]backend.ID, 0, 10)
	done := make(chan struct{})

	for _, idx := range repo.Index().All() {
		for blob := range idx.Each(done) {
			if blob.Type != pack.Tree {
				continue
			}

			list = append(list, blob.ID)
			if len(list) == cap(list) {
				close(done)
				break
			}
		}
	}

	// start benchmark
	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		for _, id := range list {
			_, err := restic.LoadTree(repo, id)
			OK(t, err)
		}
	}
}
