package restic

import "sort"

// CountedBlobSet is a set of blobs. For each blob it also stores a uint8 value
// which can be used to track some information. The CountedBlobSet does not use
// that value in any way. New entries are created with value 0.
type CountedBlobSet struct {
	m map[BlobHandle]uint8
}

// NewCountedBlobSet returns a new CountedBlobSet, populated with ids.
func NewCountedBlobSet(handles ...BlobHandle) *CountedBlobSet {
	m := CountedBlobSet{}
	m.m = make(map[BlobHandle]uint8)
	for _, h := range handles {
		m.m[h] = 0
	}

	return &m
}

func (s *CountedBlobSet) Get(h BlobHandle) (uint8, bool) {
	val, ok := s.m[h]
	return val, ok
}

func (s *CountedBlobSet) Set(h BlobHandle, value uint8) {
	s.m[h] = value
}

// Has returns true iff id is contained in the set.
func (s *CountedBlobSet) Has(h BlobHandle) bool {
	_, ok := s.m[h]
	return ok
}

// Insert adds id to the set.
func (s *CountedBlobSet) Insert(h BlobHandle) {
	s.m[h] = 0
}

// Delete removes id from the set.
func (s *CountedBlobSet) Delete(h BlobHandle) {
	delete(s.m, h)
}

func (s *CountedBlobSet) Len() int {
	return len(s.m)
}

// List returns a sorted slice of all BlobHandle in the set.
func (s *CountedBlobSet) List() BlobHandles {
	list := make(BlobHandles, 0, len(s.m))
	for h := range s.m {
		list = append(list, h)
	}

	sort.Sort(list)

	return list
}

func (s *CountedBlobSet) String() string {
	str := s.List().String()
	if len(str) < 2 {
		return "{}"
	}

	return "{" + str[1:len(str)-1] + "}"
}

// Copy returns a copy of the CountedBlobSet.
func (s *CountedBlobSet) Copy() *CountedBlobSet {
	cp := &CountedBlobSet{}
	cp.m = make(map[BlobHandle]uint8, len(s.m))
	for k, v := range s.m {
		cp.m[k] = v
	}
	return cp
}

func (s *CountedBlobSet) For(cb func(h BlobHandle, value uint8)) {
	for k, v := range s.m {
		cb(k, v)
	}
}
