package repository

import (
	"context"
	"io"
	"math/rand"
	"sync"
	"testing"

	"github.com/restic/restic/internal/repository/crypto"
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

func fillPacks(t testing.TB, rnd *rand.Rand, pm *packerManager, buf []byte) (bytes int) {
	for i := 0; i < 102; i++ {
		l := rnd.Intn(maxBlobSize)
		id := randomID(rnd)
		buf = buf[:l]
		// Only change a few bytes so we know we're not benchmarking the RNG.
		rnd.Read(buf[:min(l, 4)])

		n, err := pm.SaveBlob(context.TODO(), restic.DataBlob, id, buf, 0, 0)
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

	savedBytes := 0
	pm := newPackerManager(crypto.NewRandomKey(), restic.DataBlob, DefaultPackSize, defaultPackerCount, func(ctx context.Context, tp restic.BlobType, p *packer) error {
		err := p.Finalize()
		if err != nil {
			return err
		}
		savedBytes += int(p.Size())
		return nil
	})

	blobBuf := make([]byte, maxBlobSize)

	bytes := fillPacks(t, rnd, pm, blobBuf)
	// bytes does not include the last pack headers
	test.Assert(t, savedBytes == (bytes+36) || savedBytes == (bytes+72), "unexpected number of saved bytes, got %v, expected %v", savedBytes, bytes)

	t.Logf("saved %d bytes", bytes)
	return int64(bytes)
}

func TestPackerManagerWithOversizeBlob(t *testing.T) {
	packFiles := 0
	sizeLimit := uint(512 * 1024)
	pm := newPackerManager(crypto.NewRandomKey(), restic.DataBlob, sizeLimit, defaultPackerCount, func(ctx context.Context, tp restic.BlobType, p *packer) error {
		packFiles++
		return nil
	})

	for _, i := range []uint{sizeLimit / 2, sizeLimit, sizeLimit / 3} {
		_, err := pm.SaveBlob(context.TODO(), restic.DataBlob, restic.ID{}, make([]byte, i), 0, 0)
		test.OK(t, err)
	}
	test.OK(t, pm.Flush(context.TODO()))

	// oversized blob must be stored in a separate packfile
	test.Assert(t, packFiles == 2, "unexpected number of packfiles %v, expected 2", packFiles)
}

// TestPackerManagerGroups verifies that blobs of different groups are packed
// separately when the open-group budget allows it, that a too-small budget
// degrades gracefully to the shared bucket without losing blobs, and that no
// blobs are ever dropped. Real groups are numbered from 1; group 0 is the
// shared/fallback bucket.
func TestPackerManagerGroups(t *testing.T) {
	const (
		packSize  = uint(4096)
		blobSize  = 512
		numGroups = 4
		perGroup  = 12
	)

	// maxOpenGroups >= numGroups keeps every group in its own bucket (fully
	// pure packs); a smaller budget forces overflow groups into the shared
	// bucket 0.
	for _, tc := range []struct {
		name          string
		maxOpenGroups int
		wantPure      bool
	}{
		{"unlimited", 0, true},
		{"exact", numGroups, true},
		{"capped", 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			blobGroup := make(map[restic.ID]uint32)
			var mu sync.Mutex
			var packs [][]uint32 // per queued pack: the original groups of its blobs
			totalPacked := 0

			pm := newPackerManager(crypto.NewRandomKey(), restic.TreeBlob, packSize, 1,
				func(ctx context.Context, tp restic.BlobType, p *packer) error {
					if err := p.Finalize(); err != nil {
						return err
					}
					var groups []uint32
					for _, b := range p.Blobs() {
						g, ok := blobGroup[b.ID]
						test.Assert(t, ok, "unknown blob %v in pack", b.ID)
						groups = append(groups, g)
					}
					mu.Lock()
					packs = append(packs, groups)
					totalPacked += len(groups)
					mu.Unlock()
					return nil
				})
			pm.maxOpenGroups = tc.maxOpenGroups

			rnd := rand.New(rand.NewSource(randomSeed))
			buf := make([]byte, blobSize)
			// interleave groups so blobs of different groups are in flight together
			for i := 0; i < perGroup; i++ {
				for g := uint32(1); g <= numGroups; g++ {
					id := randomID(rnd)
					blobGroup[id] = g
					_, err := pm.SaveBlob(context.TODO(), restic.TreeBlob, id, buf, 0, g)
					test.OK(t, err)
				}
			}
			test.OK(t, pm.Flush(context.TODO()))

			// no blob may be lost, regardless of budget
			test.Equals(t, numGroups*perGroup, totalPacked)

			pureGroups := make(map[uint32]bool)
			mixedSeen := false
			for _, groups := range packs {
				g0 := groups[0]
				pure := true
				for _, g := range groups {
					if g != g0 {
						pure = false
					}
				}
				if pure {
					pureGroups[g0] = true
				} else {
					mixedSeen = true
				}
			}

			if tc.wantPure {
				// with enough budget every pack is pure and every group appears
				test.Assert(t, !mixedSeen, "unexpected pack mixing groups with budget %d", tc.maxOpenGroups)
				test.Equals(t, numGroups, len(pureGroups))
			} else {
				// with a tiny budget the overflow groups must have been merged
				// into the shared bucket, i.e. at least one mixed pack exists
				test.Assert(t, mixedSeen, "expected fallback to shared bucket with budget %d", tc.maxOpenGroups)
			}
		})
	}
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
		pm := newPackerManager(crypto.NewRandomKey(), restic.DataBlob, DefaultPackSize, defaultPackerCount, func(ctx context.Context, t restic.BlobType, p *packer) error {
			return nil
		})
		fillPacks(t, rnd, pm, blobBuf)
	}
}
