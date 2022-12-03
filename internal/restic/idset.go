package restic

import "sort"

// IDSet is a set of IDs.
type IDSet map[ID]struct{}

// NewIDSet returns a new IDSet, populated with ids.
func NewIDSet(ids ...ID) IDSet {
	m := make(IDSet)
	for _, id := range ids {
		m[id] = struct{}{}
	}

	return m
}

// Has returns true iff id is contained in the set.
func (s IDSet) Has(id ID) bool {
	_, ok := s[id]
	return ok
}

// Insert adds id to the set.
func (s IDSet) Insert(id ID) {
	s[id] = struct{}{}
}

// Delete removes id from the set.
func (s IDSet) Delete(id ID) {
	delete(s, id)
}

// List returns a sorted slice of all IDs in the set.
func (s IDSet) List() IDs {
	list := make(IDs, 0, len(s))
	for id := range s {
		list = append(list, id)
	}

	sort.Sort(list)

	return list
}

// Equals returns true iff s equals other.
func (s IDSet) Equals(other IDSet) bool {
	if len(s) != len(other) {
		return false
	}

	for id := range s {
		if _, ok := other[id]; !ok {
			return false
		}
	}

	// length + one-way comparison is sufficient implication of equality

	return true
}

// Merge adds the blobs in other to the current set.
func (s IDSet) Merge(other IDSet) {
	for id := range other {
		s.Insert(id)
	}
}

// Intersect returns a new set containing the IDs that are present in both sets.
func (s IDSet) Intersect(other IDSet) (result IDSet) {
	result = NewIDSet()

	set1 := s
	set2 := other

	// iterate over the smaller set
	if len(set2) < len(set1) {
		set1, set2 = set2, set1
	}

	for id := range set1 {
		if set2.Has(id) {
			result.Insert(id)
		}
	}

	return result
}

// Sub returns a new set containing all IDs that are present in s but not in
// other.
func (s IDSet) Sub(other IDSet) (result IDSet) {
	result = NewIDSet()
	for id := range s {
		if !other.Has(id) {
			result.Insert(id)
		}
	}

	return result
}

func (s IDSet) String() string {
	str := s.List().String()
	return "{" + str[1:len(str)-1] + "}"
}
