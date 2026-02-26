package index

import (
	"hash/maphash"
	"iter"
	"math"

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
//
// On 64-bit systems, the id of an indexEntry is stored in an uint64 in buckets
// and the next field of an indexEntry. However, the actual number of entries
// is far lower. Thus, the upper 28 bits are used to store a bloom filter,
// leaving the lower 36 bits for the index in the block list. The bloom filter
// is used to quickly check if the entry might be present in the map before
// traversing the block list. This significantly reduces the number of cache
// misses for unknown ids.
type indexMap struct {
	// The number of buckets is always a power of two and never zero.
	buckets    []uint
	numentries uint

	mh maphash.Hash

	blockList hashedArrayTree
}

const (
	maxLoad = 4 // Max. number of entries per bucket.
)

// add inserts an indexEntry for the given arguments into the map,
// using id as the key.
func (m *indexMap) add(id restic.ID, packIdx int, offset, length uint32, uncompressedLength uint32) {
	// Make sure there is enough space for the new entry.
	m.preallocate(int(m.numentries) + 1)

	h := m.hash(id)
	e, idx := m.newEntry()
	e.id = id
	e.next = m.buckets[h] // Prepend to existing chain.
	e.packIndex = packIdx
	e.offset = offset
	e.length = length
	e.uncompressedLength = uncompressedLength

	m.buckets[h] = bloomInsertID(idx, e.next, id)
	m.numentries++
}

// values returns an iterator over all entries in the map.
func (m *indexMap) values() iter.Seq[*indexEntry] {
	return func(yield func(*indexEntry) bool) {
		blockCount := m.blockList.Size()
		for i := uint(1); i < blockCount; i++ {
			if !yield(m.resolve(i)) {
				return
			}
		}
	}
}

// valuesWithID returns an iterator over all entries with the given id.
func (m *indexMap) valuesWithID(id restic.ID) iter.Seq[*indexEntry] {
	return func(yield func(*indexEntry) bool) {
		if len(m.buckets) == 0 {
			return
		}

		h := m.hash(id)
		ei := m.buckets[h]
		// checking before resolving each entry is significantly faster than
		// checking only once at the start.
		for bloomHasID(ei, id) {
			e := m.resolve(ei)
			ei = e.next
			if e.id != id {
				continue
			}
			if !yield(e) {
				return
			}
		}
	}
}

// get returns the first entry for the given id.
func (m *indexMap) get(id restic.ID) *indexEntry {
	if len(m.buckets) == 0 {
		return nil
	}

	h := m.hash(id)
	ei := m.buckets[h]
	for bloomHasID(ei, id) {
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
	for bloomHasID(ei, id) {
		e := m.resolve(ei)
		cur := bloomCleanID(ei)
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

func (m *indexMap) preallocate(numEntries int) {
	if numEntries == 0 {
		return
	}
	if len(m.buckets) == 0 {
		m.init() // Perform lazy initialization.
	}

	// new size must be a power of two
	newSize := len(m.buckets)
	for newSize < (numEntries+maxLoad-1)/maxLoad {
		newSize *= 2
	}
	if newSize == len(m.buckets) {
		return
	}

	m.buckets = make([]uint, newSize)

	blockCount := m.blockList.Size()
	for i := uint(1); i < blockCount; i++ {
		e := m.resolve(i)

		h := m.hash(e.id)
		e.next = m.buckets[h]
		m.buckets[h] = bloomInsertID(i, e.next, e.id)
	}

	m.blockList.preallocate(uint(numEntries))
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
	entry, idx := m.blockList.Alloc()
	if idx != bloomCleanID(idx) {
		panic("repository index size overflow")
	}
	return entry, idx
}

func (m *indexMap) resolve(idx uint) *indexEntry {
	return m.blockList.Ref(bloomCleanID(idx))
}

// On 32-bit systems, the bloom filter compiles away into a no-op.
const bloomShift = 36
const bloomMask = 1<<bloomShift - 1

func bloomCleanID(idx uint) uint {
	// extra variable to compile on 32bit systems
	bloomMask := uint64(bloomMask)
	return idx & uint(bloomMask)
}

func bloomForID(id restic.ID) uint {
	// A bloom filter with a single hash function seems to work best.
	// This is probably because the entry chains can be quite long, such that several entries end
	// up in the same bloom filter. In this case, a single hash function yields the lowest false positive rate.
	k1 := id[0] % (64 - bloomShift)
	return uint(1 << k1)
}

// Returns whether the idx could contain the id. Returns false only of the index cannot contain the id.
// It may return true even if the id is not present in the entry chain. However, those false positives are expected to be rare.
func bloomHasID(idx uint, id restic.ID) bool {
	if math.MaxUint == math.MaxUint32 {
		// On 32-bit systems, the bloom filter is empty for all entries.
		// Thus, simply check if there is a next entry.
		return idx != 0
	}
	bloom := idx >> bloomShift
	return bloom&bloomForID(id) != 0
}

func bloomInsertID(idx uint, nextIdx uint, id restic.ID) uint {
	// extra variable to compile on 32bit systems
	bloomMask := uint64(bloomMask)
	oldBloom := (nextIdx & ^uint(bloomMask))
	newBloom := bloomForID(id) << bloomShift
	return idx | oldBloom | newBloom
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

func (h *hashedArrayTree) preallocate(numEntries uint) {
	idx, _ := h.index(numEntries - 1)
	for int(idx) >= len(h.blockList) {
		// blockList is too short -> double list and block size
		h.blockSize *= 2
		h.mask = h.mask*2 + 1
		h.maskShift++
		idx = idx / 2

		oldBlocks := h.blockList
		h.blockList = make([][]indexEntry, h.blockSize)

		// pairwise merging of blocks
		for i := 0; i < len(oldBlocks); i += 2 {
			if oldBlocks[i] == nil && oldBlocks[i+1] == nil {
				// merged all blocks with data. Grow will allocate the block later on
				break
			}
			block := make([]indexEntry, 0, h.blockSize)
			block = append(block, oldBlocks[i]...)
			block = append(block, oldBlocks[i+1]...)
			// make sure to set the correct length as not all old blocks may contain entries yet
			h.blockList[i/2] = block[0:h.blockSize]
			// allow GC
			oldBlocks[i] = nil
			oldBlocks[i+1] = nil
		}
	}
}

func (h *hashedArrayTree) grow() {
	h.preallocate(h.size + 1)

	idx, subIdx := h.index(h.size)
	if subIdx == 0 {
		// new index entry batch
		h.blockList[idx] = make([]indexEntry, h.blockSize)
	}
}
