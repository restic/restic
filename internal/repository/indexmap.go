package repository

import (
	"crypto/rand"
	"encoding/binary"

	"github.com/restic/restic/internal/restic"

	"github.com/dchest/siphash"
)

// An indexMap is a chained hash table that maps blob IDs to indexEntries.
// It allows storing multiple entries with the same key.
//
// IndexMap uses some optimizations that are not compatible with supporting
// deletions.
//
// The buckets in this hash table contain only pointers, rather than inlined
// key-value pairs like the standard Go map. This way, only a pointer array
// needs to be resized when the table grows, preventing memory usage spikes.
type indexMap struct {
	// The number of buckets is always a power of two and never zero.
	buckets    []*indexEntry
	numentries uint

	key0, key1 uint64 // Key for hash randomization.

	free *indexEntry // Free list.
}

const (
	growthFactor = 2 // Must be a power of 2.
	maxLoad      = 4 // Max. number of entries per bucket.
)

// add inserts an indexEntry for the given arguments into the map,
// using id as the key.
func (m *indexMap) add(id restic.ID, packIdx int, offset, length uint32) {
	switch {
	case m.numentries == 0: // Lazy initialization.
		m.init()
	case m.numentries >= maxLoad*uint(len(m.buckets)):
		m.grow()
	}

	h := m.hash(id)
	e := m.newEntry()
	e.id = id
	e.next = m.buckets[h] // Prepend to existing chain.
	e.packIndex = packIdx
	e.offset = offset
	e.length = length

	m.buckets[h] = e
	m.numentries++
}

// foreach calls fn for all entries in the map, until fn returns false.
func (m *indexMap) foreach(fn func(*indexEntry) bool) {
	for _, e := range m.buckets {
		for e != nil {
			if !fn(e) {
				return
			}
			e = e.next
		}
	}
}

// foreachWithID calls fn for all entries with the given id.
func (m *indexMap) foreachWithID(id restic.ID, fn func(*indexEntry)) {
	if len(m.buckets) == 0 {
		return
	}

	h := m.hash(id)
	for e := m.buckets[h]; e != nil; e = e.next {
		if e.id != id {
			continue
		}
		fn(e)
	}
}

// get returns the first entry for the given id.
func (m *indexMap) get(id restic.ID) *indexEntry {
	if len(m.buckets) == 0 {
		return nil
	}

	h := m.hash(id)
	for e := m.buckets[h]; e != nil; e = e.next {
		if e.id == id {
			return e
		}
	}
	return nil
}

func (m *indexMap) grow() {
	old := m.buckets
	m.buckets = make([]*indexEntry, growthFactor*len(m.buckets))

	for _, e := range old {
		for e != nil {
			h := m.hash(e.id)
			next := e.next
			e.next = m.buckets[h]
			m.buckets[h] = e
			e = next
		}
	}
}

func (m *indexMap) hash(id restic.ID) uint {
	// We use siphash with a randomly generated 128-bit key, to prevent
	// backups of specially crafted inputs from degrading performance.
	// While SHA-256 should be collision-resistant, for hash table indices
	// we use only a few bits of it and finding collisions for those is
	// much easier than breaking the whole algorithm.
	h := uint(siphash.Hash(m.key0, m.key1, id[:]))
	return h & uint(len(m.buckets)-1)
}

func (m *indexMap) init() {
	const initialBuckets = 64
	m.buckets = make([]*indexEntry, initialBuckets)

	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic(err) // Very little we can do here.
	}
	m.key0 = binary.LittleEndian.Uint64(buf[:8])
	m.key1 = binary.LittleEndian.Uint64(buf[8:])
}

func (m *indexMap) len() uint { return m.numentries }

func (m *indexMap) newEntry() *indexEntry {
	// Allocating in batches means that we get closer to optimal space usage,
	// as Go's malloc will overallocate for structures of size 56 (indexEntry
	// on amd64).
	//
	// 256*56 and 256*48 both have minimal malloc overhead among reasonable sizes.
	// See src/runtime/sizeclasses.go in the standard library.
	const entryAllocBatch = 256

	if m.free == nil {
		free := new([entryAllocBatch]indexEntry)
		for i := range free[:len(free)-1] {
			free[i].next = &free[i+1]
		}
		m.free = &free[0]
	}

	e := m.free
	m.free = m.free.next

	return e
}

type indexEntry struct {
	id        restic.ID
	next      *indexEntry
	packIndex int // Position in containing Index's packs field.
	offset    uint32
	length    uint32
}
