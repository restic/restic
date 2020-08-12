package repository

import (
	"context"
	"io"
	"math/rand"
	"os"
	"sync"
	"testing"

	"github.com/restic/restic/internal/backend/mem"
	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/mock"
	"github.com/restic/restic/internal/restic"
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

func saveFile(t testing.TB, be Saver, length int, f *os.File, id restic.ID) {
	h := restic.Handle{Type: restic.PackFile, Name: id.String()}
	t.Logf("save file %v", h)

	rd, err := restic.NewFileReader(f)
	if err != nil {
		t.Fatal(err)
	}

	err = be.Save(context.TODO(), h, rd)
	if err != nil {
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	if err := fs.RemoveIfExists(f.Name()); err != nil {
		t.Fatal(err)
	}
}

func fillPacks(t testing.TB, rnd *rand.Rand, be Saver, pm *packerManager, buf []byte) (bytes int) {
	for i := 0; i < 100; i++ {
		l := rnd.Intn(maxBlobSize)

		packer, err := pm.findPacker()
		if err != nil {
			t.Fatal(err)
		}

		id := randomID(rnd)
		buf = buf[:l]
		// Only change a few bytes so we know we're not benchmarking the RNG.
		rnd.Read(buf[:min(l, 4)])

		n, err := packer.Add(restic.DataBlob, id, buf)
		if err != nil {
			t.Fatal(err)
		}
		if n != l {
			t.Errorf("Add() returned invalid number of bytes: want %v, got %v", n, l)
		}
		bytes += l

		if packer.Size() < minPackSize {
			pm.insertPacker(packer)
			continue
		}

		_, err = packer.Finalize()
		if err != nil {
			t.Fatal(err)
		}

		packID := restic.IDFromHash(packer.hw.Sum(nil))
		saveFile(t, be, int(packer.Size()), packer.tmpfile, packID)
	}

	return bytes
}

func flushRemainingPacks(t testing.TB, be Saver, pm *packerManager) (bytes int) {
	if pm.countPacker() > 0 {
		for _, packer := range pm.packers {
			n, err := packer.Finalize()
			if err != nil {
				t.Fatal(err)
			}
			bytes += int(n)

			packID := restic.IDFromHash(packer.hw.Sum(nil))
			saveFile(t, be, int(packer.Size()), packer.tmpfile, packID)
		}
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

	be := mem.New()
	pm := newPackerManager(be, crypto.NewRandomKey())

	blobBuf := make([]byte, maxBlobSize)

	bytes := fillPacks(t, rnd, be, pm, blobBuf)
	bytes += flushRemainingPacks(t, be, pm)

	t.Logf("saved %d bytes", bytes)
	return int64(bytes)
}

func BenchmarkPackerManager(t *testing.B) {
	// Run testPackerManager if it hasn't run already, to set totalSize.
	once.Do(func() {
		totalSize = testPackerManager(t)
	})

	rnd := rand.New(rand.NewSource(randomSeed))

	be := &mock.Backend{
		SaveFn: func(context.Context, restic.Handle, restic.RewindReader) error { return nil },
	}
	blobBuf := make([]byte, maxBlobSize)

	t.ReportAllocs()
	t.SetBytes(totalSize)
	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		rnd.Seed(randomSeed)
		pm := newPackerManager(be, crypto.NewRandomKey())
		fillPacks(t, rnd, be, pm, blobBuf)
		flushRemainingPacks(t, be, pm)
	}
}
