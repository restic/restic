package restic_test

import (
	"math/rand"
	"testing"

	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
)

func TestZeroPrefixLen(t *testing.T) {
	var buf [2048]byte

	// test zero prefixes of various lengths
	for i := len(buf) - 1; i >= 0; i-- {
		buf[i] = 42
		skipped := restic.ZeroPrefixLen(buf[:])
		test.Equals(t, i, skipped)
	}
	// test buffers of various sizes
	for i := 0; i < len(buf); i++ {
		skipped := restic.ZeroPrefixLen(buf[i:])
		test.Equals(t, 0, skipped)
	}
}

func BenchmarkZeroPrefixLen(b *testing.B) {
	var (
		buf        [4<<20 + 37]byte
		r          = rand.New(rand.NewSource(0x618732))
		sumSkipped int64
	)

	b.ReportAllocs()
	b.SetBytes(int64(len(buf)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		j := r.Intn(len(buf))
		buf[j] = 0xff

		skipped := restic.ZeroPrefixLen(buf[:])
		sumSkipped += int64(skipped)

		buf[j] = 0
	}

	// The closer this is to .5, the better. If it's far off, give the
	// benchmark more time to run with -benchtime.
	b.Logf("average number of zeros skipped: %.3f",
		float64(sumSkipped)/(float64(b.N*len(buf))))
}
