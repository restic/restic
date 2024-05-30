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

	blockList hashedArrayTree
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
	blockCount := m.blockList.Size()
	for i := uint(1); i < blockCount; i++ {
		if !fn(m.resolve(i)) {
			return
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

// firstIndex returns the index of the first entry for ID id.
// This index is guaranteed to never change.
func (m *indexMap) firstIndex(id restic.ID) int {
	if len(m.buckets) == 0 {
		return -1
	}

	idx := -1
	h := m.hash(id)
	ei := m.buckets[h]
	for ei != 0 {
		e := m.resolve(ei)
		cur := ei
		ei = e.next
		if e.id != id {
			continue
		}
		if int(cur) < idx || idx == -1 {
			// casting from uint to int is unproblematic as we'd run out of memory
			// before this can result in an overflow.
			idx = int(cur)
		}
	}
	return idx
}

func (m *indexMap) grow() {
	m.buckets = make([]uint, growthFactor*len(m.buckets))

	blockCount := m.blockList.Size()
	for i := uint(1); i < blockCount; i++ {
		e := m.resolve(i)

		h := m.hash(e.id)
		e.next = m.buckets[h]
		m.buckets[h] = i
	}
}

func (m *indexMap) hash(id restic.ID) uint {
	// We use maphash to prevent backups of specially crafted inputs
	// from degrading performance.
	// While SHA-256 should be collision-resistant, for hash table indices
	// we use only a few bits of it and finding collisions for those is
	// much easier than breaking the whole algorithm.
	mh := maphash.Hash{}
	mh.SetSeed(m.mh.Seed())
	_, _ = mh.Write(id[:])
	h := uint(mh.Sum64())
	return h & uint(len(m.buckets)-1)
}

func (m *indexMap) init() {
	const initialBuckets = 64
	m.buckets = make([]uint, initialBuckets)
	// first entry in blockList serves as null byte
	m.blockList = *newHAT()
	m.newEntry()
}

func (m *indexMap) len() uint { return m.numentries }

func (m *indexMap) newEntry() (*indexEntry, uint) {
	return m.blockList.Alloc()
}

func (m *indexMap) resolve(idx uint) *indexEntry {
	return m.blockList.Ref(idx)
}

type indexEntry struct {
	id                 restic.ID
	next               uint
	packIndex          int // Position in containing Index's packs field.
	offset             uint32
	length             uint32
	uncompressedLength uint32
}

type hashedArrayTree struct {
	mask      uint
	maskShift uint
	blockSize uint

	size      uint
	blockList [][]indexEntry
}

func newHAT() *hashedArrayTree {
	// start with a small block size
	blockSizePower := uint(2)
	blockSize := uint(1 << blockSizePower)

	return &hashedArrayTree{
		mask:      blockSize - 1,
		maskShift: blockSizePower,
		blockSize: blockSize,
		size:      0,
		blockList: make([][]indexEntry, blockSize),
	}
}

func (h *hashedArrayTree) Alloc() (*indexEntry, uint) {
	h.grow()
	size := h.size
	idx, subIdx := h.index(size)
	h.size++
	return &h.blockList[idx][subIdx], size
}

func (h *hashedArrayTree) index(pos uint) (idx uint, subIdx uint) {
	subIdx = pos & h.mask
	idx = pos >> h.maskShift
	return
}

func (h *hashedArrayTree) Ref(pos uint) *indexEntry {
	if pos >= h.size {
		panic("array index out of bounds")
	}

	idx, subIdx := h.index(pos)
	return &h.blockList[idx][subIdx]
}

func (h *hashedArrayTree) Size() uint {
	return h.size
}

func (h *hashedArrayTree) grow() {
	idx, subIdx := h.index(h.size)
	if int(idx) == len(h.blockList) {
		// blockList is too short -> double list and block size
		h.blockSize *= 2
		h.mask = h.mask*2 + 1
		h.maskShift++
		idx = idx / 2

		oldBlocks := h.blockList
		h.blockList = make([][]indexEntry, h.blockSize)

		// pairwise merging of blocks
		for i := 0; i < len(oldBlocks); i += 2 {
			block := make([]indexEntry, 0, h.blockSize)
			block = append(block, oldBlocks[i]...)
			block = append(block, oldBlocks[i+1]...)
			h.blockList[i/2] = block
			// allow GC
			oldBlocks[i] = nil
			oldBlocks[i+1] = nil
		}
	}
	if subIdx == 0 {
		// new index entry batch
		h.blockList[idx] = make([]indexEntry, h.blockSize)
	}
}
