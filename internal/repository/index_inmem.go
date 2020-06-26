package repository

import (
	"sort"

	"github.com/restic/restic/internal/restic"
)

// Low Memory Map
//
// is a low memory replacement for the standard map[key]value.
// In order to get low memory consumption, we use large buckets which have in
// average about 1000 to 2000 entries.
// The entries of bucket are saved in a paged array, that is an array of "pages"
// which each contain up to 32 entries.
// As each bucket usually consists of 32 to 64 pages this gives an average
// unsed page space of only 1/2 page out of 48 pages. The organizational overhead
// is 8 bytes per page and 48 bytes per bucket on 64bit architectures.
// Hence the organizational overhead per entry is less than 1/2 byte per map entry.
//
// Note that duplicate keys are allowed and stored separately.
// This low memory map must be sorted using the sort method before it can be
// accessed.
//
//
// As there are no generics in go, we use the types mapKey and mapValue
// which have to be defined in order to use a generic implementation.

// Define type of key and values of the map
// please ensure that mapKey implements Hash, Less, LessEqual and Equal
type mapKey restic.ID
type mapValue indexEntry

func (key mapKey) Hash() int {
	var x int
	var s uint
	id := restic.ID(key)
	for _, b := range id[:8] {
		x |= int(b) << s
		s += 8
	}

	return x
}

func (key mapKey) Less(other mapKey) bool {
	return restic.ID(key).Less(restic.ID(other))
}

func (key mapKey) LessEqual(other mapKey) bool {
	return restic.ID(key).LessEqual(restic.ID(other))
}

func (key mapKey) Equal(other mapKey) bool {
	return restic.ID(key).Equal(restic.ID(other))
}

// --- start of generic implementation

// grow when each bucket has in average 64 pages
const GrowFactor = 64

// Make pages of 2^5 = 32 blobEntries by default
const shiftBits = 5
const pageSize = 1 << shiftBits
const maskBits = pageSize - 1

// lowMemMap - actually behaves like map[mapKey]mapEntry, but uses much less memory
type lowMemMap struct {
	cnt       int
	bucketCnt int
	mask      int

	buckets []*bucket
}

// newLowMemMap creates a new low memory hashmap
func newLowMemMap() *lowMemMap {
	buckets := make([]*bucket, 1)
	buckets[0] = newBucket()
	return &lowMemMap{
		mask:      0,
		buckets:   buckets,
		bucketCnt: 0,
	}
}

// get(k) returns the value for the given key k
// entry, found := sm.get(k) works like entry, found := map[k]
func (m *lowMemMap) get(key mapKey) (mapValue, bool) {
	bucket := m.buckets[key.Hash()&m.mask]
	return bucket.getKey(key)
}

// getAll(k) returns all values (including duplicates) for the given key k
func (m *lowMemMap) getAll(key mapKey) []mapValue {
	bucket := m.buckets[key.Hash()&m.mask]
	return bucket.getAllKey(key)
}

// len returns the number of entries in the hashmap
func (m *lowMemMap) len() int {
	return m.cnt
}

// forEach calls the given func for all entries of the hashmap
func (m *lowMemMap) forEach(fn func(mapKey, mapValue)) {
	for _, b := range m.buckets {
		b.forEach(fn)
	}
}

// clear frees all memory used for the hashmap
func (m *lowMemMap) clear() {
	for i := range m.buckets {
		m.buckets[i].clear()
		m.buckets[i] = nil
	}
	m.buckets = nil
}

// sort sorts all buckets in the hashmap
func (m *lowMemMap) sort() {
	// TODO: this can be done concurrently
	for _, b := range m.buckets {
		b.sort()
	}
}

// add adds an entry to the hashmap
func (m *lowMemMap) add(key mapKey, entry mapValue) {
	bucket := m.buckets[key.Hash()&m.mask]
	m.cnt++
	if bucket.append(bucketEntry{key: key, value: entry}) {
		m.bucketCnt++
		if m.bucketCnt >= GrowFactor*len(m.buckets) {
			m.grow()
		}
	}
}

// grow grows the hashmap and thus doubles the number of buckets
func (m *lowMemMap) grow() {
	oldLen := len(m.buckets)
	newBuckets := make([]*bucket, 2*oldLen)
	m.mask = 2*m.mask + 1
	// TODO: this can be done concurrently
	for i, b := range m.buckets {
		newBuckets[i] = b
		newBucket, delta := b.split(func(key mapKey) bool {
			return (key.Hash() & m.mask) == i+oldLen
		})
		newBuckets[i+oldLen] = newBucket
		m.bucketCnt += delta
	}
	m.buckets = newBuckets
}

// bucket implements a "paged" array of bucketEntry.
// It is a replacement for []bucketEntry.
// The difference to a simple array is that it uses fix "pages" of a
// given length.
//
// Using a paged array makes bucket faster for append as no existing elements
// need to be copied but instead only new "pages" are added.
// Also the memory overhead is at most one "page" and hence significantly
// smaller than of a standard array for big arrays.
// The cost is a more expensive access to elements.
type bucket struct {
	page   []bucketPage
	length int
	sorted bool
}

type bucketPage *[pageSize]bucketEntry

type bucketEntry struct {
	key   mapKey
	value mapValue
}

// newBucket creates a new bucket
func newBucket() *bucket {
	return &bucket{}
}

// newPage creates a new page for a bucket
func newPage() bucketPage {
	return &[pageSize]bucketEntry{}
}

func (b *bucket) clear() {
	b.page = nil
}

// splitIndex is a helper function to split i into inner and outer index
func splitIndex(i int) (int, int) {
	return i >> shiftBits, i & maskBits
}

// Let bucket implement sort.Interface
func (b *bucket) Len() int {
	return b.length
}
func (b *bucket) Less(i, j int) bool {
	return b.get(i).key.Less(b.get(j).key)
}
func (b *bucket) Swap(i, j int) {
	// calculate outer and inner indices
	outI, inI := splitIndex(i)
	outJ, inJ := splitIndex(j)

	b.page[outI][inI], b.page[outJ][inJ] =
		b.page[outJ][inJ], b.page[outI][inI]
}

func (b *bucket) sort() {
	if b.sorted {
		return
	}
	sort.Sort(b)
	b.sorted = true
}

// get accesses an element of the bucket; entry = b.get(i) is the eqivalent of entry = b[i]
func (b *bucket) get(i int) bucketEntry {
	outI, inI := splitIndex(i)
	return b.page[outI][inI]
}

// append appends an entry; b.append(entry) is the equivalent of b = append(b, entry)
// it returns whether we needed to add a page to the bucket
func (b *bucket) append(blob bucketEntry) (pageAdded bool) {
	outI, inI := splitIndex(b.length)
	// if page is full, create a new page
	if inI == 0 {
		b.page = append(b.page, newPage())
		pageAdded = true
	}
	b.page[outI][inI] = blob
	b.length++
	b.sorted = false

	return pageAdded
}

// forEach calls the given function for all entries in the bucket
func (bt *bucket) forEach(fn func(mapKey, mapValue)) {
	for k := 0; k < bt.length; k++ {
		be := bt.get(k)
		fn(be.key, be.value)
	}
}

// getKey searches for an elements with given key; bucket must be sorted!
// returns the first found entry and wheter found or not
func (b *bucket) getKey(key mapKey) (mapValue, bool) {
	if !b.sorted {
		panic("bucket is not sorted!")
	}
	if k := sort.Search(b.length, func(i int) bool {
		return key.LessEqual(b.get(i).key)
	}); k < b.length {
		if be := b.get(k); be.key.Equal(key) {
			return be.value, true
		}
	}
	return mapValue{}, false
}

// getAllKey returns all elements with given ID; bucket must be sorted!
func (bt *bucket) getAllKey(key mapKey) (entries []mapValue) {
	if !bt.sorted {
		panic("bucket is not sorted!")
	}
	for k := sort.Search(bt.length, func(i int) bool {
		return key.LessEqual(bt.get(i).key)
	}); k < bt.length; k++ {
		be := bt.get(k)
		if !be.key.Equal(key) {
			break
		}
		entries = append(entries, be.value)
	}
	return entries
}

// split splits a bucket into two depending on a given split function
// if splitFn is true for a key, the entry will be moved to a newly created bucket
// returns the newly created bucket and the number of pages which have been added
func (bt *bucket) split(splitFn func(mapKey) bool) (newbucket *bucket, pagesAdded int) {
	newbucket = newBucket()
	newbucket.sorted = bt.sorted
	var outJ, inJ, newLength int

	for i := 0; i < bt.length; i++ {
		entry := bt.get(i)
		if splitFn(entry.key) {
			if newbucket.append(entry) {
				pagesAdded++
			}
		} else {
			if inJ >= pageSize {
				outJ++
				inJ = 0
			}
			bt.page[outJ][inJ] = entry
			inJ++
			newLength++
		}
	}
	bt.length = newLength

	// remove unneeded pages
	for i := outJ + 1; i < len(bt.page); i++ {
		bt.page[i] = nil
		pagesAdded--
	}
	bt.page = bt.page[:outJ+1]

	return newbucket, pagesAdded
}
