package restic_test

import (
	"bytes"
	"flag"
	"io"
	"testing"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/chunker"
	"github.com/restic/restic/pack"
	"github.com/restic/restic/server"
	. "github.com/restic/restic/test"
)

var benchArchiveDirectory = flag.String("test.benchdir", ".", "benchmark archiving a real directory (default: .)")
var testPol = chunker.Pol(0x3DA3358B4DC173)

const bufSize = chunker.MiB

type Rdr interface {
	io.ReadSeeker
	io.ReaderAt
}

func benchmarkChunkEncrypt(b testing.TB, buf, buf2 []byte, rd Rdr, key *server.Key) {
	ch := restic.GetChunker("BenchmarkChunkEncrypt")
	rd.Seek(0, 0)
	ch.Reset(rd, testPol)

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

		_, err = key.Encrypt(buf2, buf)
		OK(b, err)
	}

	restic.FreeChunker("BenchmarkChunkEncrypt", ch)
}

func BenchmarkChunkEncrypt(b *testing.B) {
	data := Random(23, 10<<20) // 10MiB
	rd := bytes.NewReader(data)

	s := SetupBackend(b)
	defer TeardownBackend(b, s)

	buf := restic.GetChunkBuf("BenchmarkChunkEncrypt")
	buf2 := restic.GetChunkBuf("BenchmarkChunkEncrypt")

	b.ResetTimer()
	b.SetBytes(int64(len(data)))

	for i := 0; i < b.N; i++ {
		benchmarkChunkEncrypt(b, buf, buf2, rd, s.Key())
	}

	restic.FreeChunkBuf("BenchmarkChunkEncrypt", buf)
	restic.FreeChunkBuf("BenchmarkChunkEncrypt", buf2)
}

func benchmarkChunkEncryptP(b *testing.PB, buf []byte, rd Rdr, key *server.Key) {
	ch := restic.GetChunker("BenchmarkChunkEncryptP")
	rd.Seek(0, 0)
	ch.Reset(rd, testPol)

	for {
		chunk, err := ch.Next()
		if err == io.EOF {
			break
		}

		// reduce length of chunkBuf
		buf = buf[:chunk.Length]
		io.ReadFull(chunk.Reader(rd), buf)
		key.Encrypt(buf, buf)
	}

	restic.FreeChunker("BenchmarkChunkEncryptP", ch)
}

func BenchmarkChunkEncryptParallel(b *testing.B) {
	s := SetupBackend(b)
	defer TeardownBackend(b, s)

	data := Random(23, 10<<20) // 10MiB

	buf := restic.GetChunkBuf("BenchmarkChunkEncryptParallel")

	b.ResetTimer()
	b.SetBytes(int64(len(data)))

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rd := bytes.NewReader(data)
			benchmarkChunkEncryptP(pb, buf, rd, s.Key())
		}
	})

	restic.FreeChunkBuf("BenchmarkChunkEncryptParallel", buf)
}

func archiveDirectory(b testing.TB) {
	server := SetupBackend(b)
	defer TeardownBackend(b, server)

	arch := restic.NewArchiver(server)

	_, id, err := arch.Snapshot(nil, []string{*benchArchiveDirectory}, nil)
	OK(b, err)

	b.Logf("snapshot archived as %v", id)
}

func TestArchiveDirectory(t *testing.T) {
	if *benchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping TestArchiveDirectory")
	}

	archiveDirectory(t)
}

func BenchmarkArchiveDirectory(b *testing.B) {
	if *benchArchiveDirectory == "" {
		b.Skip("benchdir not set, skipping BenchmarkArchiveDirectory")
	}

	for i := 0; i < b.N; i++ {
		archiveDirectory(b)
	}
}

func archiveWithDedup(t testing.TB) {
	if *benchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping TestArchiverDedup")
	}

	server := SetupBackend(t)
	defer TeardownBackend(t, server)

	var cnt struct {
		before, after, after2 struct {
			packs, dataBlobs, treeBlobs uint
		}
	}

	// archive a few files
	sn := SnapshotDir(t, server, *benchArchiveDirectory, nil)
	t.Logf("archived snapshot %v", sn.ID().Str())

	// get archive stats
	cnt.before.packs = server.Count(backend.Data)
	cnt.before.dataBlobs = server.Index().Count(pack.Data)
	cnt.before.treeBlobs = server.Index().Count(pack.Tree)
	t.Logf("packs %v, data blobs %v, tree blobs %v",
		cnt.before.packs, cnt.before.dataBlobs, cnt.before.treeBlobs)

	// archive the same files again, without parent snapshot
	sn2 := SnapshotDir(t, server, *benchArchiveDirectory, nil)
	t.Logf("archived snapshot %v", sn2.ID().Str())

	// get archive stats again
	cnt.after.packs = server.Count(backend.Data)
	cnt.after.dataBlobs = server.Index().Count(pack.Data)
	cnt.after.treeBlobs = server.Index().Count(pack.Tree)
	t.Logf("packs %v, data blobs %v, tree blobs %v",
		cnt.after.packs, cnt.after.dataBlobs, cnt.after.treeBlobs)

	// if there are more packs or blobs, something is wrong
	if cnt.after.packs > cnt.before.packs {
		t.Fatalf("TestArchiverDedup: too many packs in repository: before %d, after %d",
			cnt.before.packs, cnt.after.packs)
	}
	if cnt.after.dataBlobs > cnt.before.dataBlobs {
		t.Fatalf("TestArchiverDedup: too many data blobs in repository: before %d, after %d",
			cnt.before.dataBlobs, cnt.after.dataBlobs)
	}
	if cnt.after.treeBlobs > cnt.before.treeBlobs {
		t.Fatalf("TestArchiverDedup: too many tree blobs in repository: before %d, after %d",
			cnt.before.treeBlobs, cnt.after.treeBlobs)
	}

	// archive the same files again, with a parent snapshot
	sn3 := SnapshotDir(t, server, *benchArchiveDirectory, sn2.ID())
	t.Logf("archived snapshot %v, parent %v", sn3.ID().Str(), sn2.ID().Str())

	// get archive stats again
	cnt.after2.packs = server.Count(backend.Data)
	cnt.after2.dataBlobs = server.Index().Count(pack.Data)
	cnt.after2.treeBlobs = server.Index().Count(pack.Tree)
	t.Logf("packs %v, data blobs %v, tree blobs %v",
		cnt.after2.packs, cnt.after2.dataBlobs, cnt.after2.treeBlobs)

	// if there are more packs or blobs, something is wrong
	if cnt.after2.packs > cnt.before.packs {
		t.Fatalf("TestArchiverDedup: too many packs in repository: before %d, after %d",
			cnt.before.packs, cnt.after2.packs)
	}
	if cnt.after2.dataBlobs > cnt.before.dataBlobs {
		t.Fatalf("TestArchiverDedup: too many data blobs in repository: before %d, after %d",
			cnt.before.dataBlobs, cnt.after2.dataBlobs)
	}
	if cnt.after2.treeBlobs > cnt.before.treeBlobs {
		t.Fatalf("TestArchiverDedup: too many tree blobs in repository: before %d, after %d",
			cnt.before.treeBlobs, cnt.after2.treeBlobs)
	}
}

func TestArchiveDedup(t *testing.T) {
	archiveWithDedup(t)
}

func BenchmarkLoadTree(t *testing.B) {
	if *benchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping TestArchiverDedup")
	}

	s := SetupBackend(t)
	defer TeardownBackend(t, s)

	// archive a few files
	arch := restic.NewArchiver(s)
	sn, _, err := arch.Snapshot(nil, []string{*benchArchiveDirectory}, nil)
	OK(t, err)
	t.Logf("archived snapshot %v", sn.ID())

	list := make([]backend.ID, 0, 10)
	done := make(chan struct{})

	for blob := range s.Index().Each(done) {
		if blob.Type != pack.Tree {
			continue
		}

		list = append(list, blob.ID)
		if len(list) == cap(list) {
			close(done)
			break
		}
	}

	// start benchmark
	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		for _, id := range list {
			_, err := restic.LoadTree(s, id)
			OK(t, err)
		}
	}
}
