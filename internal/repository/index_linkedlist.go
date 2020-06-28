package repository

// Linked List Map
//
// Note that duplicate keys are allowed and stored separately.
//
//
// As there are no generics in go, we use the types mapKey and mapValue
// which have to be defined in order to use a generic implementation.

// --- start of generic implementation

const maxEntriesPerBucket = 8

// linkedMemMap - actually behaves like map[mapKey]mapEntry, but uses much less memory
type linkedMemMap struct {
	cnt  int
	mask int

	buckets []*linkedBucket
}

type linkedBucket struct {
	key   mapKey
	value mapValue
	next  *linkedBucket
}

// newLinkedMemMap creates a new low memory hashmap
func newLinkedMemMap() *linkedMemMap {
	buckets := make([]*linkedBucket, 1)
	return &linkedMemMap{
		mask:    0,
		buckets: buckets,
	}
}

// get(k) returns the value for the given key k
// entry, found := sm.get(k) works like entry, found := map[k]
func (m *linkedMemMap) get(key mapKey) (mapValue, bool) {
	bucket := m.buckets[key.Hash()&m.mask]
	for bucket != nil {
		if bucket.key.Compare(key) == 0 {
			return bucket.value, true
		}
		bucket = bucket.next
	}
	return mapValue{}, false
}

// getAll(k) returns all values (including duplicates) for the given key k
func (m *linkedMemMap) getAll(key mapKey) (values []mapValue) {
	bucket := m.buckets[key.Hash()&m.mask]
	for bucket != nil {
		if bucket.key.Compare(key) == 0 {
			values = append(values, bucket.value)
		}
		bucket = bucket.next
	}
	return values
}

// len returns the number of entries in the hashmap
func (m *linkedMemMap) len() int {
	return m.cnt
}

// forEach calls the given func for all entries of the hashmap
func (m *linkedMemMap) forEach(fn func(mapKey, mapValue)) {
	for _, b := range m.buckets {
		for b != nil {
			fn(b.key, b.value)
			b = b.next
		}
	}
}

func (m *linkedMemMap) sort() {
}

// clear frees all memory used for the hashmap
func (m *linkedMemMap) clear() {
	for i := range m.buckets {
		m.buckets[i] = nil
	}
	m.buckets = nil
}

// add adds an entry to the hashmap
func (m *linkedMemMap) add(key mapKey, entry mapValue) {
	index := key.Hash() & m.mask
	b := &linkedBucket{key: key,
		value: entry,
		next:  m.buckets[index],
	}
	m.buckets[index] = b

	m.cnt++
	if m.cnt >= maxEntriesPerBucket*len(m.buckets) {
		m.grow()
	}
}

// grow grows the hashmap and thus doubles the number of buckets
func (m *linkedMemMap) grow() {
	newBuckets := make([]*linkedBucket, 2*len(m.buckets))
	m.mask = 2*m.mask + 1
	// TODO: this can be done concurrently
	for _, b := range m.buckets {
		for b != nil {
			index := b.key.Hash() & m.mask
			next := b.next
			b.next = newBuckets[index]
			newBuckets[index] = b
			b = next
		}
	}
	m.buckets = newBuckets
}
