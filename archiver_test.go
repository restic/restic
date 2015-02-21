package restic_test

import (
	"bytes"
	"flag"
	"io"
	"math/rand"
	"testing"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/chunker"
)

var benchArchiveDirectory = flag.String("test.benchdir", "", "benchmark archiving a real directory")

func get_random(seed, count int) []byte {
	buf := make([]byte, count)

	rnd := rand.New(rand.NewSource(int64(seed)))
	for i := 0; i < count; i++ {
		buf[i] = byte(rnd.Uint32())
	}

	return buf
}

func randomReader(seed, size int) *bytes.Reader {
	return bytes.NewReader(get_random(seed, size))
}

const bufSize = chunker.MiB

type Rdr interface {
	io.ReadSeeker
	io.ReaderAt
}

func benchmarkChunkEncrypt(b testing.TB, buf []byte, rd Rdr, key *restic.Key) {
	ch := restic.GetChunker("BenchmarkChunkEncrypt")
	rd.Seek(0, 0)
	ch.Reset(rd)

	for {
		chunk, err := ch.Next()

		if err == io.EOF {
			break
		}

		ok(b, err)

		// reduce length of buf
		buf = buf[:chunk.Length]
		n, err := io.ReadFull(chunk.Reader(rd), buf)
		ok(b, err)
		assert(b, uint(n) == chunk.Length, "invalid length: got %d, expected %d", n, chunk.Length)

		_, err = key.Encrypt(buf, buf)
		ok(b, err)
	}

	restic.FreeChunker("BenchmarkChunkEncrypt", ch)
}

func BenchmarkChunkEncrypt(b *testing.B) {
	data := get_random(23, 10<<20) // 10MiB
	rd := bytes.NewReader(data)

	be := setupBackend(b)
	defer teardownBackend(b, be)
	key := setupKey(b, be, "geheim")

	buf := restic.GetChunkBuf("BenchmarkChunkEncrypt")

	b.ResetTimer()
	b.SetBytes(int64(len(data)))

	for i := 0; i < b.N; i++ {
		benchmarkChunkEncrypt(b, buf, rd, key)
	}

	restic.FreeChunkBuf("BenchmarkChunkEncrypt", buf)
}

func benchmarkChunkEncryptP(b *testing.PB, buf []byte, rd Rdr, key *restic.Key) {
	ch := restic.GetChunker("BenchmarkChunkEncryptP")
	rd.Seek(0, 0)
	ch.Reset(rd)

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

	data := get_random(23, 10<<20) // 10MiB

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

	be := setupBackend(b)
	defer teardownBackend(b, be)
	key := setupKey(b, be, "geheim")
	server := restic.NewServerWithKey(be, key)

	arch, err := restic.NewArchiver(server)
	ok(b, err)

	_, id, err := arch.Snapshot(nil, *benchArchiveDirectory, nil)

	b.Logf("snapshot archived as %v", id)
}

func snapshot(t testing.TB, server restic.Server, path string) *restic.Snapshot {
	arch, err := restic.NewArchiver(server)
	ok(t, err)
	ok(t, arch.Preload(nil))
	sn, _, err := arch.Snapshot(nil, path, nil)
	ok(t, err)
	return sn
}

func countBlobs(t testing.TB, server restic.Server) int {
	blobs := 0
	err := server.EachID(backend.Tree, func(id backend.ID) {
		tree, err := restic.LoadTree(server, id)
		ok(t, err)

		blobs += tree.Map.Len()
	})
	ok(t, err)

	return blobs
}

func archiveWithPreload(t testing.TB) {
	if *benchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping TestArchiverPreload")
	}

	be := setupBackend(t)
	defer teardownBackend(t, be)
	key := setupKey(t, be, "geheim")
	server := restic.NewServerWithKey(be, key)

	// archive a few files
	sn := snapshot(t, server, *benchArchiveDirectory)
	t.Logf("archived snapshot %v", sn.ID())

	// get archive stats
	blobsBefore := countBlobs(t, server)
	t.Logf("found %v blobs", blobsBefore)

	// archive the same files again
	sn2 := snapshot(t, server, *benchArchiveDirectory)
	t.Logf("archived snapshot %v", sn2.ID())

	// get archive stats
	blobsAfter := countBlobs(t, server)
	t.Logf("found %v blobs", blobsAfter)

	// if there are more than 50% more blobs, something is wrong
	if blobsAfter > (blobsBefore + blobsBefore/2) {
		t.Fatalf("TestArchiverPreload: too many blobs in repository: before %d, after %d, threshhold %d",
			blobsBefore, blobsAfter, (blobsBefore + blobsBefore/2))
	}
}

func TestArchivePreload(t *testing.T) {
	archiveWithPreload(t)
}

func BenchmarkArchivePreload(b *testing.B) {
	archiveWithPreload(b)
}

func BenchmarkPreload(t *testing.B) {
	if *benchArchiveDirectory == "" {
		t.Skip("benchdir not set, skipping TestArchiverPreload")
	}

	be := setupBackend(t)
	defer teardownBackend(t, be)
	key := setupKey(t, be, "geheim")
	server := restic.NewServerWithKey(be, key)

	// archive a few files
	arch, err := restic.NewArchiver(server)
	ok(t, err)
	sn, _, err := arch.Snapshot(nil, *benchArchiveDirectory, nil)
	ok(t, err)
	t.Logf("archived snapshot %v", sn.ID())

	// start benchmark
	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		// create new archiver and preload
		arch2, err := restic.NewArchiver(server)
		ok(t, err)
		ok(t, arch2.Preload(nil))
	}
}
