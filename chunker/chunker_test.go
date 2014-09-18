package chunker_test

import (
	"bytes"
	"io"
	"math/rand"
	"testing"

	"github.com/fd0/khepri/chunker"
)

type chunk struct {
	Length int
	CutFP  uint64
}

// created for 32MB of random data out of math/rand's Uint32() seeded by
// constant 23
//
// chunking configuration:
// window size 64, avg chunksize 1<<20, min chunksize 1<<19, max chunksize 1<<23
// polynom 0x3DA3358B4DC173
var chunks1 = []chunk{
	chunk{2163460, 0x000b98d4cdf00000},
	chunk{643703, 0x000d4e8364d00000},
	chunk{1528956, 0x0015a25c2ef00000},
	chunk{1955808, 0x00102a8242e00000},
	chunk{2222372, 0x00045da878000000},
	chunk{2538687, 0x00198a8179900000},
	chunk{609606, 0x001d4e8d17100000},
	chunk{1205738, 0x000a7204dd600000},
	chunk{959742, 0x00183e71e1400000},
	chunk{4036109, 0x001fec043c700000},
	chunk{1525894, 0x000b1574b1500000},
	chunk{1352720, 0x00018965f2e00000},
	chunk{811884, 0x00155628aa100000},
	chunk{1282314, 0x001909a0a1400000},
	chunk{1318021, 0x001cceb980000000},
	chunk{948640, 0x0011f7a470a00000},
	chunk{645464, 0x00030ce2d9400000},
	chunk{533758, 0x0004435c53c00000},
	chunk{1128303, 0x0000c48517800000},
	chunk{800374, 0x000968473f900000},
	chunk{2453512, 0x001e197c92600000},
	chunk{2651975, 0x000ae6c868000000},
	chunk{237392, 0x00184c5825e18636},
}

func test_with_data(t *testing.T, chunker chunker.Chunker, chunks []chunk) {
	for i, chunk := range chunks {
		c, err := chunker.Next()

		if err != nil {
			t.Fatalf("Error returned with chunk %d: %v", i, err)
		}

		if c == nil {
			t.Fatalf("Nil chunk returned")
		}

		if c != nil {
			if c.Length != chunk.Length {
				t.Fatalf("Length for chunk %d does not match: expected %d, got %d",
					i, chunk.Length, c.Length)
			}

			if len(c.Data) != chunk.Length {
				t.Fatalf("Data length for chunk %d does not match: expected %d, got %d",
					i, chunk.Length, len(c.Data))
			}

			if c.Cut != chunk.CutFP {
				t.Fatalf("Cut fingerprint for chunk %d does not match: expected %016x, got %016x",
					i, chunk.CutFP, c.Cut)
			}
		}
	}

	c, err := chunker.Next()

	if c != nil {
		t.Fatal("additional non-nil chunk returned")
	}

	if err != io.EOF {
		t.Fatal("wrong error returned after last chunk")
	}
}

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

func TestChunker(t *testing.T) {
	// setup data source
	buf := get_random(23, 32*1024*1024)

	ch := chunker.New(bytes.NewReader(buf))

	test_with_data(t, ch, chunks1)
}

func BenchmarkChunker(b *testing.B) {
	size := 10 * 1024 * 1024
	buf := get_random(23, size)

	b.ResetTimer()
	b.SetBytes(int64(size))
	var chunks int
	for i := 0; i < b.N; i++ {
		chunks = 0

		ch := chunker.New(bytes.NewReader(buf))

		for {
			_, err := ch.Next()

			if err == io.EOF {
				break
			}

			if err != nil {
				b.Fatalf("Unexpected error occurred: %v", err)
			}

			chunks++
		}
	}

	b.Logf("%d chunks, average chunk size: %d bytes", chunks, size/chunks)
}
