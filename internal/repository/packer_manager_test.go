package repository

import (
	"context"
	"io"
	"math/rand"
	"sync"
	"testing"

	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
)

func randomID(rd io.Reader) restic.ID {
	id := restic.ID{}
	_, err := io.ReadFull(rd, id[:])
	if err != nil {
		panic(err)
	}
	return id
}

const maxBlobSize = 1 << 20

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func fillPacks(t testing.TB, rnd *rand.Rand, pm *packerManager, buf []byte) (bytes int) {
	for i := 0; i < 102; i++ {
		l := rnd.Intn(maxBlobSize)
		id := randomID(rnd)
		buf = buf[:l]
		// Only change a few bytes so we know we're not benchmarking the RNG.
		rnd.Read(buf[:min(l, 4)])

		n, err := pm.SaveBlob(context.TODO(), restic.DataBlob, id, buf, 0)
		if err != nil {
			t.Fatal(err)
		}
		if n != l+37 && n != l+37+36 {
			t.Errorf("Add() returned invalid number of bytes: want %v, got %v", l, n)
		}
		bytes += n
	}
	err := pm.Flush(context.TODO())
	if err != nil {
		t.Fatal(err)
	}
	return bytes
}

const randomSeed = 23

var (
	once      sync.Once
	totalSize int64
)

func TestPackerManager(t *testing.T) {
	bytes := testPackerManager(t)
	once.Do(func() { totalSize = bytes })
}

func testPackerManager(t testing.TB) int64 {
	rnd := rand.New(rand.NewSource(randomSeed))

	savedBytes := int(0)
	pm := newPackerManager(crypto.NewRandomKey(), restic.DataBlob, DefaultPackSize, func(ctx context.Context, tp restic.BlobType, p *Packer) error {
		err := p.Finalize()
		if err != nil {
			return err
		}
		savedBytes += int(p.Size())
		return nil
	})

	blobBuf := make([]byte, maxBlobSize)

	bytes := fillPacks(t, rnd, pm, blobBuf)
	// bytes does not include the last packs header
	test.Equals(t, savedBytes, bytes+36)

	t.Logf("saved %d bytes", bytes)
	return int64(bytes)
}

func BenchmarkPackerManager(t *testing.B) {
	// Run testPackerManager if it hasn't run already, to set totalSize.
	once.Do(func() {
		totalSize = testPackerManager(t)
	})

	rnd := rand.New(rand.NewSource(randomSeed))
	blobBuf := make([]byte, maxBlobSize)

	t.ReportAllocs()
	t.SetBytes(totalSize)
	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		rnd.Seed(randomSeed)
		pm := newPackerManager(crypto.NewRandomKey(), restic.DataBlob, DefaultPackSize, func(ctx context.Context, t restic.BlobType, p *Packer) error {
			return nil
		})
		fillPacks(t, rnd, pm, blobBuf)
	}
}
