package khepri_test

import (
	"bytes"
	"io"
	"math/rand"
	"testing"

	"github.com/fd0/khepri"
	"github.com/fd0/khepri/chunker"
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

func BenchmarkChunkEncrypt(b *testing.B) {
	data := get_random(23, 10<<20) // 10MiB

	be := setupBackend(b)
	defer teardownBackend(b, be)
	key := setupKey(b, be, "geheim")
	chunkBuf := make([]byte, chunker.MaxSize)

	b.ResetTimer()
	b.SetBytes(int64(len(data)))

	for i := 0; i < b.N; i++ {
		ch := chunker.New(bytes.NewReader(data))

		for {
			chunk_data, err := ch.Next(chunkBuf)

			if err == io.EOF {
				break
			}

			ok(b, err)

			buf := make([]byte, khepri.CiphertextExtension+chunker.MaxSize)
			_, err = key.Encrypt(buf, chunk_data.Data)
			ok(b, err)
		}
	}
}
