package restic

import "sort"

// BlobSet is a set of blobs.
type BlobSet struct {
	byType [NumBlobTypes]IDSet
}

// NewBlobSet returns a new blobSet, populated with blob handles.
// Note that this is not thread-safe, but writing to blobs of different types
// concurrently is safe.
func NewBlobSet(handles ...BlobHandle) BlobSet {
	var m BlobSet
	for t := InvalidBlob; t < NumBlobTypes; t++ {
		m.byType[t] = NewIDSet()
	}
	for _, h := range handles {
		m.byType[h.Type].Insert(h.ID)
	}

	return m
}

// ForAll calls the given func for all entries in the blobSet
func (s BlobSet) ForAll(fn func(h BlobHandle) error) error {
	for t := InvalidBlob; t < NumBlobTypes; t++ {
		for id := range s.byType[t] {
			err := fn(BlobHandle{Type: t, ID: id})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Len returns the number of elements in the blobSet
func (s BlobSet) Len() (length int) {
	for t := InvalidBlob; t < NumBlobTypes; t++ {
		length += len(s.byType[t])
	}
	return length
}

// Has returns true iff id is contained in the set.
func (s BlobSet) Has(h BlobHandle) bool {
	return s.byType[h.Type].Has(h.ID)
}

// Insert adds id to the set.
func (s BlobSet) Insert(h BlobHandle) {
	s.byType[h.Type].Insert(h.ID)
}

// Delete removes id from the set.
func (s BlobSet) Delete(h BlobHandle) {
	delete(s.byType[h.Type], h.ID)
}

// Equals returns true iff s equals other.
func (s BlobSet) Equals(other BlobSet) bool {
	for t := InvalidBlob; t < NumBlobTypes; t++ {
		if !s.byType[t].Equals(other.byType[t]) {
			return false
		}
	}

	return true
}

// Merge adds the blobs in other to the current set.
func (s BlobSet) Merge(other BlobSet) {
	for t := InvalidBlob; t < NumBlobTypes; t++ {
		for id := range other.byType[t] {
			s.byType[t].Insert(id)
		}
	}
}

// Intersect returns a new set containing the handles that are present in both sets.
func (s BlobSet) Intersect(other BlobSet) BlobSet {
	result := NewBlobSet()

	set1 := s
	set2 := other

	for t := InvalidBlob; t < NumBlobTypes; t++ {
		// iterate over the smaller set
		if len(set2.byType[t]) < len(set1.byType[t]) {
			set1, set2 = set2, set1
		}

		for id := range set1.byType[t] {
			if set2.byType[t].Has(id) {
				result.byType[t].Insert(id)
			}
		}
	}

	return result
}

// Sub returns a new set containing all handles that are present in s but not in
// other.
func (s BlobSet) Sub(other BlobSet) BlobSet {
	result := NewBlobSet()
	for t := InvalidBlob; t < NumBlobTypes; t++ {
		for id := range s.byType[t] {
			if !other.byType[t].Has(id) {
				result.byType[t].Insert(id)
			}
		}
	}

	return result
}

// List returns a sorted slice of all BlobHandle in the set.
func (s BlobSet) List() BlobHandles {
	list := make(BlobHandles, 0, s.Len())
	for t := InvalidBlob; t < NumBlobTypes; t++ {
		for id := range s.byType[t] {
			list = append(list, BlobHandle{Type: t, ID: id})
		}
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
