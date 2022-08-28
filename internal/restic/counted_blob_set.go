package restic

import "sort"

// CountedBlobSet is a set of blobs. For each blob it also stores a uint8 value
// which can be used to track some information. The CountedBlobSet does not use
// that value in any way. New entries are created with value 0.
type CountedBlobSet map[BlobHandle]uint8

// NewCountedBlobSet returns a new CountedBlobSet, populated with ids.
func NewCountedBlobSet(handles ...BlobHandle) CountedBlobSet {
	m := make(CountedBlobSet)
	for _, h := range handles {
		m[h] = 0
	}

	return m
}

// Has returns true iff id is contained in the set.
func (s CountedBlobSet) Has(h BlobHandle) bool {
	_, ok := s[h]
	return ok
}

// Insert adds id to the set.
func (s CountedBlobSet) Insert(h BlobHandle) {
	s[h] = 0
}

// Delete removes id from the set.
func (s CountedBlobSet) Delete(h BlobHandle) {
	delete(s, h)
}

func (s CountedBlobSet) Len() int {
	return len(s)
}

// List returns a sorted slice of all BlobHandle in the set.
func (s CountedBlobSet) List() BlobHandles {
	list := make(BlobHandles, 0, len(s))
	for h := range s {
		list = append(list, h)
	}

	sort.Sort(list)

	return list
}

func (s CountedBlobSet) String() string {
	str := s.List().String()
	if len(str) < 2 {
		return "{}"
	}

	return "{" + str[1:len(str)-1] + "}"
}

// Copy returns a copy of the CountedBlobSet.
func (s CountedBlobSet) Copy() CountedBlobSet {
	cp := make(CountedBlobSet, len(s))
	for k, v := range s {
		cp[k] = v
	}
	return cp
}
