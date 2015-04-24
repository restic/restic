package restic_test

import (
	"bytes"
	"flag"
	"io"
	"testing"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/chunker"
	. "github.com/restic/restic/test"
)

var benchArchiveDirectory = flag.String("test.benchdir", ".", "benchmark archiving a real directory (default: .)")
var testPol = chunker.Pol(0x3DA3358B4DC173)

const bufSize = chunker.MiB

type Rdr interface {
	io.ReadSeeker
	io.ReaderAt
}

func benchmarkChunkEncrypt(b testing.TB, buf, buf2 []byte, rd Rdr, key *restic.Key) {
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

	be := setupBackend(b)
	defer teardownBackend(b, be)
	key := setupKey(b, be, "geheim")

	buf := restic.GetChunkBuf("BenchmarkChunkEncrypt")
	buf2 := restic.GetChunkBuf("BenchmarkChunkEncrypt")

	b.ResetTimer()
	b.SetBytes(int64(len(data)))

	for i := 0; i < b.N; i++ {
		benchmarkChunkEncrypt(b, buf, buf2, rd, key)
	}

	restic.FreeChunkBuf("BenchmarkChunkEncrypt", buf)
	restic.FreeChunkBuf("BenchmarkChunkEncrypt", buf2)
}

func benchmarkChunkEncryptP(b *testing.PB, buf []byte, rd Rdr, key *restic.Key) {
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
	be := setupBackend(b)
	defer teardownBackend(b, be)
	key := setupKey(b, be, "geheim")

	data := Random(23, 10<<20) // 10MiB

	buf := restic.GetChunkBuf("BenchmarkChunkEncryptParallel")

	b.ResetTimer()
	b.SetBytes(int64(len(data)))

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rd := bytes.NewReader(data)
			benchmarkChunkEncryptP(pb, buf, rd, key)
		}
	})

	restic.FreeChunkBuf("BenchmarkChunkEncryptParallel", buf)
}

func BenchmarkArchiveDirectory(b *testing.B) {
	if *benchArchiveDirectory == "" {
		b.Skip("benchdir not set, skipping BenchmarkArchiveDirectory")
	}

	server := setupBackend(b)
	defer teardownBackend(b, server)
	key := setupKey(b, server, "geheim")
	server.SetKey(key)

	arch, err := restic.NewArchiver(server)
	OK(b, err)

	_, id, err := arch.Snapshot(nil, []string{*benchArchiveDirectory}, nil)

	b.Logf("snapshot archived as %v", id)
}

func snapshot(t testing.TB, server restic.Server, path string, parent backend.ID) *restic.Snapshot {
	arch, err := restic.NewArchiver(server)
	OK(t, err)
	OK(t, arch.Preload())
	sn, _, err := arch.Snapshot(nil, []string{path}, parent)
	OK(t, err)
	return sn
}

func countBlobs(t testing.TB, server restic.Server) (trees int, data int) {
	return server.Count(backend.Tree), server.Count(backend.Data)
}

func archiveWithPreload(t testing.TB) {
	if *benchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping TestArchiverPreload")
	}

	server := setupBackend(t)
	defer teardownBackend(t, server)
	key := setupKey(t, server, "geheim")
	server.SetKey(key)

	// archive a few files
	sn := snapshot(t, server, *benchArchiveDirectory, nil)
	t.Logf("archived snapshot %v", sn.ID().Str())

	// get archive stats
	beforeTrees, beforeData := countBlobs(t, server)
	t.Logf("found %v trees, %v data blobs", beforeTrees, beforeData)

	// archive the same files again, without parent snapshot
	sn2 := snapshot(t, server, *benchArchiveDirectory, nil)
	t.Logf("archived snapshot %v", sn2.ID().Str())

	// get archive stats
	afterTrees2, afterData2 := countBlobs(t, server)
	t.Logf("found %v trees, %v data blobs", afterTrees2, afterData2)

	// if there are more blobs, something is wrong
	if afterData2 > beforeData {
		t.Fatalf("TestArchiverPreload: too many data blobs in repository: before %d, after %d",
			beforeData, afterData2)
	}

	// archive the same files again, with a parent snapshot
	sn3 := snapshot(t, server, *benchArchiveDirectory, sn2.ID())
	t.Logf("archived snapshot %v, parent %v", sn3.ID().Str(), sn2.ID().Str())

	// get archive stats
	afterTrees3, afterData3 := countBlobs(t, server)
	t.Logf("found %v trees, %v data blobs", afterTrees3, afterData3)

	// if there are more blobs, something is wrong
	if afterData3 > beforeData {
		t.Fatalf("TestArchiverPreload: too many data blobs in repository: before %d, after %d",
			beforeData, afterData3)
	}
}

func TestArchivePreload(t *testing.T) {
	archiveWithPreload(t)
}

func BenchmarkPreload(t *testing.B) {
	if *benchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping TestArchiverPreload")
	}

	server := setupBackend(t)
	defer teardownBackend(t, server)
	key := setupKey(t, server, "geheim")
	server.SetKey(key)

	// archive a few files
	arch, err := restic.NewArchiver(server)
	OK(t, err)
	sn, _, err := arch.Snapshot(nil, []string{*benchArchiveDirectory}, nil)
	OK(t, err)
	t.Logf("archived snapshot %v", sn.ID())

	// start benchmark
	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		// create new archiver and preload
		arch2, err := restic.NewArchiver(server)
		OK(t, err)
		OK(t, arch2.Preload())
	}
}

func BenchmarkLoadTree(t *testing.B) {
	if *benchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping TestArchiverPreload")
	}

	server := setupBackend(t)
	defer teardownBackend(t, server)
	key := setupKey(t, server, "geheim")
	server.SetKey(key)

	// archive a few files
	arch, err := restic.NewArchiver(server)
	OK(t, err)
	sn, _, err := arch.Snapshot(nil, []string{*benchArchiveDirectory}, nil)
	OK(t, err)
	t.Logf("archived snapshot %v", sn.ID())

	list := make([]backend.ID, 0, 10)
	done := make(chan struct{})

	for name := range server.List(backend.Tree, done) {
		id, err := backend.ParseID(name)
		if err != nil {
			t.Logf("invalid id for tree %v", name)
			continue
		}

		list = append(list, id)
		if len(list) == cap(list) {
			close(done)
			break
		}
	}

	// start benchmark
	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		for _, id := range list {
			_, err := restic.LoadTree(server, restic.Blob{Storage: id})
			OK(t, err)
		}
	}
}
