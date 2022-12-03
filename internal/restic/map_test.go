package restic

import (
	"math/rand"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func TestMapBasic(t *testing.T) {
	t.Parallel()

	var (
		bh BlobHandle
		m  Map[struct{}]
		r  = rand.New(rand.NewSource(1234567))
	)

	for i := 1; i <= 400; i++ {
		r.Read(bh.ID[:])
		rtest.Assert(t, m.Get(bh) == nil, "%v retrieved but not added", bh)

		m.Add(bh)
		rtest.Assert(t, m.Get(bh) != nil, "%v added but not retrieved", bh)
		rtest.Equals(t, uint(i), m.Len())
	}
}

func TestMapForeach(t *testing.T) {
	t.Parallel()

	const N = 10

	var m Map[int]

	// Don't panic on empty map.
	m.Foreach(func(*MapEntry[int], BlobType) bool { return true })

	for i := 0; i < N; i++ {
		var bh BlobHandle
		bh.ID[0] = byte(i)
		m.Add(bh).Data = i
	}

	seen := make(map[int]struct{})
	m.Foreach(func(e *MapEntry[int], _ BlobType) bool {
		i := int(e.ID[0])
		rtest.Assert(t, i < N, "unknown handle %v in Map", e.ID)
		rtest.Equals(t, i, i)

		seen[i] = struct{}{}
		return true
	})

	rtest.Equals(t, N, len(seen))

	ncalls := 0
	m.Foreach(func(*MapEntry[int], BlobType) bool {
		ncalls++
		return false
	})
	rtest.Equals(t, 1, ncalls)
}

func TestMapForeachWithID(t *testing.T) {
	t.Parallel()

	const ndups = 3

	var (
		bh BlobHandle
		m  Map[int]
		r  = rand.New(rand.NewSource(1234321))
	)
	r.Read(bh.ID[:])

	// No result (and no panic) for empty map.
	n := 0
	m.ForeachWithID(bh, func(*MapEntry[int]) { n++ })
	rtest.Equals(t, 0, n)

	// Test insertion and retrieval of duplicates.
	for i := 0; i < ndups; i++ {
		m.Add(bh).Data = i
	}

	for i := 0; i < 100; i++ {
		var other BlobHandle
		r.Read(other.ID[:])
		m.Add(other).Data = -1
	}

	n = 0
	var vals [ndups]bool
	m.ForeachWithID(bh, func(e *MapEntry[int]) {
		vals[e.Data] = true
		n++
	})
	rtest.Equals(t, ndups, n)

	for i := range vals {
		rtest.Assert(t, vals[i], "duplicate with value %d not retrieved", i)
	}
}
