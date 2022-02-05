package index

import (
	"hash/maphash"

	"github.com/restic/restic/internal/restic"
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
	buckets    []uint
	numentries uint

	mh maphash.Hash

	blockList []indexEntry
}

const (
	growthFactor = 2 // Must be a power of 2.
	maxLoad      = 4 // Max. number of entries per bucket.
)

// add inserts an indexEntry for the given arguments into the map,
// using id as the key.
func (m *indexMap) add(id restic.ID, packIdx int, offset, length uint32, uncompressedLength uint32) {
	switch {
	case m.numentries == 0: // Lazy initialization.
		m.init()
	case m.numentries >= maxLoad*uint(len(m.buckets)):
		m.grow()
	}

	h := m.hash(id)
	e, idx := m.newEntry()
	e.id = id
	e.next = m.buckets[h] // Prepend to existing chain.
	e.packIndex = packIdx
	e.offset = offset
	e.length = length
	e.uncompressedLength = uncompressedLength

	m.buckets[h] = idx
	m.numentries++
}

// foreach calls fn for all entries in the map, until fn returns false.
func (m *indexMap) foreach(fn func(*indexEntry) bool) {
	for _, ei := range m.buckets {
		for ei != 0 {
			e := m.resolve(ei)
			if !fn(e) {
				return
			}
			ei = e.next
		}
	}
}

// foreachWithID calls fn for all entries with the given id.
func (m *indexMap) foreachWithID(id restic.ID, fn func(*indexEntry)) {
	if len(m.buckets) == 0 {
		return
	}

	h := m.hash(id)
	ei := m.buckets[h]
	for ei != 0 {
		e := m.resolve(ei)
		ei = e.next
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
	ei := m.buckets[h]
	for ei != 0 {
		e := m.resolve(ei)
		if e.id == id {
			return e
		}
		ei = e.next
	}
	return nil
}

func (m *indexMap) grow() {
	old := m.buckets
	m.buckets = make([]uint, growthFactor*len(m.buckets))

	for _, ei := range old {
		for ei != 0 {
			e := m.resolve(ei)
			h := m.hash(e.id)
			next := e.next
			e.next = m.buckets[h]
			m.buckets[h] = ei
			ei = next
		}
	}
}

func (m *indexMap) hash(id restic.ID) uint {
	// We use maphash to prevent backups of specially crafted inputs
	// from degrading performance.
	// While SHA-256 should be collision-resistant, for hash table indices
	// we use only a few bits of it and finding collisions for those is
	// much easier than breaking the whole algorithm.
	m.mh.Reset()
	_, _ = m.mh.Write(id[:])
	h := uint(m.mh.Sum64())
	return h & uint(len(m.buckets)-1)
}

func (m *indexMap) init() {
	const initialBuckets = 64
	m.buckets = make([]uint, initialBuckets)
	// first entry in blockList serves as null byte
	m.blockList = make([]indexEntry, 1)
}

func (m *indexMap) len() uint { return m.numentries }

func (m *indexMap) newEntry() (*indexEntry, uint) {
	m.blockList = append(m.blockList, indexEntry{})

	idx := uint(len(m.blockList) - 1)
	e := &m.blockList[idx]

	return e, idx
}

func (m *indexMap) resolve(idx uint) *indexEntry {
	return &m.blockList[idx]
}

type indexEntry struct {
	id                 restic.ID
	next               uint
	packIndex          int // Position in containing Index's packs field.
	offset             uint32
	length             uint32
	uncompressedLength uint32
}
