package restic_test

import (
	"bytes"
	"flag"
	"io"
	"math/rand"
	"testing"

	"github.com/restic/restic"
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

	arch, err := restic.NewArchiver(server, nil)
	ok(b, err)

	_, id, err := arch.Snapshot(*benchArchiveDirectory, nil)

	b.Logf("snapshot archived as %v", id)
}
