package restic

import "sort"

// BlobSet is a set of blobs.
type BlobSet map[BlobHandle]struct{}

// NewBlobSet returns a new BlobSet, populated with ids.
func NewBlobSet(handles ...BlobHandle) BlobSet {
	m := make(BlobSet)
	for _, h := range handles {
		m[h] = struct{}{}
	}

	return m
}

// Has returns true iff id is contained in the set.
func (s BlobSet) Has(h BlobHandle) bool {
	_, ok := s[h]
	return ok
}

// Insert adds id to the set.
func (s BlobSet) Insert(h BlobHandle) {
	s[h] = struct{}{}
}

// Delete removes id from the set.
func (s BlobSet) Delete(h BlobHandle) {
	delete(s, h)
}

func (s BlobSet) Len() int {
	return len(s)
}

// Equals returns true iff s equals other.
func (s BlobSet) Equals(other BlobSet) bool {
	if len(s) != len(other) {
		return false
	}

	for h := range s {
		if _, ok := other[h]; !ok {
			return false
		}
	}

	return true
}

// Merge adds the blobs in other to the current set.
func (s BlobSet) Merge(other BlobSet) {
	for h := range other {
		s.Insert(h)
	}
}

// Intersect returns a new set containing the handles that are present in both sets.
func (s BlobSet) Intersect(other BlobSet) (result BlobSet) {
	result = NewBlobSet()

	set1 := s
	set2 := other

	// iterate over the smaller set
	if len(set2) < len(set1) {
		set1, set2 = set2, set1
	}

	for h := range set1 {
		if set2.Has(h) {
			result.Insert(h)
		}
	}

	return result
}

// Sub returns a new set containing all handles that are present in s but not in
// other.
func (s BlobSet) Sub(other BlobSet) (result BlobSet) {
	result = NewBlobSet()
	for h := range s {
		if !other.Has(h) {
			result.Insert(h)
		}
	}

	return result
}

// List returns a sorted slice of all BlobHandle in the set.
func (s BlobSet) List() BlobHandles {
	list := make(BlobHandles, 0, len(s))
	for h := range s {
		list = append(list, h)
	}

	sort.Sort(list)

	return list
}

func (s BlobSet) String() string {
	str := s.List().String()
	if len(str) < 2 {
		return "{}"
	}

	return "{" + str[1:len(str)-1] + "}"
}
