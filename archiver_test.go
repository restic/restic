package restic_test

import (
	"bytes"
	"io"
	"math/rand"
	"testing"

	"github.com/restic/restic"
	"github.com/restic/restic/chunker"
)

func get_random(seed, count int) []byte {
	buf := make([]byte, count)

	rnd := rand.New(rand.NewSource(23))
	for i := 0; i < count; i += 4 {
		r := rnd.Uint32()
		buf[i] = byte(r)
		buf[i+1] = byte(r >> 8)
		buf[i+2] = byte(r >> 16)
		buf[i+3] = byte(r >> 24)
	}

	return buf
}

const bufSize = chunker.MiB

func benchmarkChunkEncrypt(b testing.TB, rd interface {
	io.ReadSeeker
	io.ReaderAt
}, key *restic.Key) {
	chunkBuf := make([]byte, restic.CiphertextExtension+chunker.MaxSize)

	ch := restic.GetChunker("BenchmarkChunkEncrypt")
	rd.Seek(0, 0)
	ch.Reset(rd)

	for {
		chunk, err := ch.Next()

		if err == io.EOF {
			break
		}

		ok(b, err)

		// reduce length of chunkBuf
		chunkBuf = chunkBuf[:chunk.Length]
		n, err := io.ReadFull(chunk.Reader(rd), chunkBuf)
		ok(b, err)
		assert(b, uint(n) == chunk.Length, "invalid length: got %d, expected %d", n, chunk.Length)

		_, err = key.Encrypt(chunkBuf, chunkBuf)
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

	b.ResetTimer()
	b.SetBytes(int64(len(data)))

	for i := 0; i < b.N; i++ {
		benchmarkChunkEncrypt(b, rd, key)
	}
}

func benchmarkChunkEncryptP(b *testing.PB, rd interface {
	io.ReadSeeker
	io.ReaderAt
}, key *restic.Key) {
	chunkBuf := make([]byte, restic.CiphertextExtension+chunker.MaxSize)

	ch := restic.GetChunker("BenchmarkChunkEncryptP")
	rd.Seek(0, 0)
	ch.Reset(rd)

	for {
		chunk, err := ch.Next()
		if err == io.EOF {
			break
		}

		// reduce length of chunkBuf
		chunkBuf = chunkBuf[:chunk.Length]
		io.ReadFull(chunk.Reader(rd), chunkBuf)
		key.Encrypt(chunkBuf, chunkBuf)
	}

	restic.FreeChunker("BenchmarkChunkEncryptP", ch)
}

func BenchmarkChunkEncryptParallel(b *testing.B) {
	be := setupBackend(b)
	defer teardownBackend(b, be)
	key := setupKey(b, be, "geheim")

	data := get_random(23, 10<<20) // 10MiB

	b.ResetTimer()
	b.SetBytes(int64(len(data)))

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rd := bytes.NewReader(data)
			benchmarkChunkEncryptP(pb, rd, key)
		}
	})
}
