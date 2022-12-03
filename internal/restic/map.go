package restic

import "hash/maphash"

// A Map is a mapping of BlobHandle to V.
// There may be multiple entries for a given handle.
type Map[V any] struct {
	byType [NumBlobTypes]idmap[V]

	mh maphash.Hash

	free *MapEntry[V] // Free list.
}

// An idmap is a chaining hash table that maps blob IDs to values of type V.
// It allows storing multiple entries with the same key.
//
// Map uses some optimizations that are not compatible with supporting
// deletions.
//
// The buckets in this hash table contain only pointers, rather than inlined
// key-value pairs like the standard Go map. This way, only a pointer array
// needs to be resized when the table grows, preventing memory usage spikes.
type idmap[V any] struct {
	// The number of buckets is always a power of two and never zero.
	buckets    []*MapEntry[V]
	numentries uint
}

// A MapEntry is an entry in a Map with an ID and a value V.
// There may be multiple entries for a given ID.
type MapEntry[V any] struct {
	ID   ID // Key.
	Data V  // Satellite data. Not touched by Map.
	next *MapEntry[V]
}

const (
	growthFactor = 2 // Must be a power of 2.
	maxLoad      = 4 // Max. number of entries per bucket.
)

// Add creates a new MapEntry for h in the map and returns it.
func (m *Map[V]) Add(h BlobHandle) *MapEntry[V] {
	tm := &m.byType[h.Type]

	switch {
	case tm.numentries == 0: // Lazy initialization.
		tm.init()
	case tm.numentries >= maxLoad*uint(len(tm.buckets)):
		m.grow(tm)
	}

	hash := tm.hindex(m.hash(h.ID))
	e := m.newEntry()
	e.ID = h.ID
	e.next = tm.buckets[hash] // Prepend to existing chain.

	tm.buckets[hash] = e
	tm.numentries++

	return e
}

// Foreach calls fn for all entries in the map, until fn returns false.
// Callers may modify a MapEntry's V, but not its ID.
func (m *Map[V]) Foreach(fn func(*MapEntry[V], BlobType) bool) {
	for typ := range m.byType {
		for _, e := range m.byType[typ].buckets {
			for e != nil {
				if !fn(e, BlobType(typ)) {
					return
				}
				e = e.next
			}
		}
	}
}

// ForeachWithID calls fn for all entries with the given h.
// Callers may modify an entry's V, but not its ID.
func (m *Map[V]) ForeachWithID(h BlobHandle, fn func(*MapEntry[V])) {
	tm := &m.byType[h.Type]
	if len(tm.buckets) == 0 {
		return
	}

	hash := tm.hindex(m.hash(h.ID))
	for e := tm.buckets[hash]; e != nil; e = e.next {
		if e.ID != h.ID {
			continue
		}
		fn(e)
	}
}

// Get returns the first entry for the given id.
func (m *Map[V]) Get(h BlobHandle) *MapEntry[V] {
	tm := &m.byType[h.Type]
	if len(tm.buckets) == 0 {
		return nil
	}

	hash := tm.hindex(m.hash(h.ID))
	for e := tm.buckets[hash]; e != nil; e = e.next {
		if e.ID == h.ID {
			return e
		}
	}
	return nil
}

func (m *Map[V]) grow(tm *idmap[V]) {
	old := tm.buckets
	tm.buckets = make([]*MapEntry[V], growthFactor*len(old))

	for _, e := range old {
		for e != nil {
			h := tm.hindex(m.hash(e.ID))
			next := e.next
			e.next = tm.buckets[h]
			tm.buckets[h] = e
			e = next
		}
	}
}

func (m *Map[V]) hash(id ID) uint {
	// We use maphash to prevent backups of specially crafted inputs
	// from degrading performance.
	// While SHA-256 should be collision-resistant, for hash table indices
	// we use only a few bits of it and finding collisions for those is
	// much easier than breaking the whole algorithm.
	m.mh.Reset()
	_, _ = m.mh.Write(id[:])
	return uint(m.mh.Sum64())
}

// hindex returns an index based on a hash from Map.hash.
func (m *idmap[V]) hindex(h uint) uint { return h & uint(len(m.buckets)-1) }

func (m *idmap[V]) init() {
	const initialBuckets = 64
	m.buckets = make([]*MapEntry[V], initialBuckets)
}

func (m *Map[V]) Len() (n uint) {
	for typ := range m.byType {
		n += m.byType[typ].numentries
	}
	return n
}

func (m *Map[V]) newEntry() *MapEntry[V] {
	// We keep a free list of objects to speed up allocation and GC.
	// There's an obvious trade-off here: allocating in larger batches
	// means we allocate faster and the GC has to keep fewer bits to track
	// what we have in use, but it means we waste some space.
	//
	// The current batch size is tuned for idmap's main client,
	// internal/index, which has a rather large V to store.
	//
	// Allocating each MapEntry[index.mapValue] separately wastes space
	// on 32-bit platforms, because the Go malloc has no size class for
	// exactly 52 bytes, so it puts the MapEntry in a 64-byte slot instead.
	// See src/runtime/sizeclasses.go in the Go source repo.
	//
	// The batch size of 4 means we hit the size classes for 4×64=256 bytes
	// (64-bit) and 4×52=208 bytes (32-bit), wasting nothing in malloc on
	// 64-bit and relatively little on 32-bit.
	const entryAllocBatch = 4

	e := m.free
	if e != nil {
		m.free = e.next
	} else {
		free := new([entryAllocBatch]MapEntry[V])
		e = &free[0]
		for i := 1; i < len(free)-1; i++ {
			free[i].next = &free[i+1]
		}
		m.free = &free[1]
	}

	return e
}
