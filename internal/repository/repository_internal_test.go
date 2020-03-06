package repository

import (
	"math/rand"
	"testing"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

type mapcache map[restic.Handle]struct{}

func (c mapcache) Has(h restic.Handle) bool {
	_, ok := c[h]
	return ok
}

func TestSortCachedPacksFirst(t *testing.T) {
	var (
		blobs   [100]restic.PackedBlob
		blobset = make(map[restic.PackedBlob]struct{})
		cache   = make(mapcache)
		r       = rand.New(rand.NewSource(1261))
	)

	for i := 0; i < len(blobs); i++ {
		var id restic.ID
		r.Read(id[:])
		blobs[i] = restic.PackedBlob{PackID: id}
		blobset[blobs[i]] = struct{}{}

		if i%3 == 0 {
			h := restic.Handle{Name: id.String(), Type: restic.DataFile}
			cache[h] = struct{}{}
		}
	}

	sorted := sortCachedPacksFirst(cache, blobs[:])

	rtest.Equals(t, len(blobs), len(sorted))
	for i := 0; i < len(blobs); i++ {
		h := restic.Handle{Type: restic.DataFile, Name: sorted[i].PackID.String()}
		if i < len(cache) {
			rtest.Assert(t, cache.Has(h), "non-cached blob at front of sorted output")
		} else {
			rtest.Assert(t, !cache.Has(h), "cached blob at end of sorted output")
		}
		_, ok := blobset[sorted[i]]
		rtest.Assert(t, ok, "sortCachedPacksFirst changed blob id")
	}
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
			h := restic.Handle{Name: id.String(), Type: restic.DataFile}
			cache[h] = struct{}{}
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
