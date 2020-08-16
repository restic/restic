package repository

import (
	"math/rand"
	"sort"
	"testing"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

type mapcache map[restic.Handle]bool

func (c mapcache) Has(h restic.Handle) bool { return c[h] }

func TestSortCachedPacksFirst(t *testing.T) {
	var (
		blobs, sorted [100]restic.PackedBlob

		cache = make(mapcache)
		r     = rand.New(rand.NewSource(1261))
	)

	for i := 0; i < len(blobs); i++ {
		var id restic.ID
		r.Read(id[:])
		blobs[i] = restic.PackedBlob{PackID: id}

		if i%3 == 0 {
			h := restic.Handle{Name: id.String(), Type: restic.PackFile}
			cache[h] = true
		}
	}

	copy(sorted[:], blobs[:])
	sort.SliceStable(sorted[:], func(i, j int) bool {
		hi := restic.Handle{Type: restic.PackFile, Name: sorted[i].PackID.String()}
		hj := restic.Handle{Type: restic.PackFile, Name: sorted[j].PackID.String()}
		return cache.Has(hi) && !cache.Has(hj)
	})

	sortCachedPacksFirst(cache, blobs[:])
	rtest.Equals(t, sorted, blobs)
}

func BenchmarkSortCachedPacksFirst(b *testing.B) {
	const nblobs = 512 // Corresponds to a file of ca. 2GB.

	var (
		blobs [nblobs]restic.PackedBlob
		cache = make(mapcache)
		r     = rand.New(rand.NewSource(1261))
	)

	for i := 0; i < nblobs; i++ {
		var id restic.ID
		r.Read(id[:])
		blobs[i] = restic.PackedBlob{PackID: id}

		if i%3 == 0 {
			h := restic.Handle{Name: id.String(), Type: restic.PackFile}
			cache[h] = true
		}
	}

	var cpy [nblobs]restic.PackedBlob
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		copy(cpy[:], blobs[:])
		sortCachedPacksFirst(cache, cpy[:])
	}
}
