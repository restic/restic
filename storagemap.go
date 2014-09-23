package khepri

import (
	"bytes"
	"sort"

	"github.com/fd0/khepri/backend"
)

type StorageMap Blobs

func NewStorageMap() *StorageMap {
	return &StorageMap{}
}

func (m StorageMap) find(id backend.ID) (int, *Blob) {
	i := sort.Search(len(m), func(i int) bool {
		return bytes.Compare(m[i].ID, id) >= 0
	})

	if i < len(m) && bytes.Equal(m[i].ID, id) {
		return i, m[i]
	}

	return i, nil
}

func (m StorageMap) Find(id backend.ID) *Blob {
	_, blob := m.find(id)
	return blob
}

func (m *StorageMap) Insert(blob *Blob) {
	pos, b := m.find(blob.ID)
	if b != nil {
		// already present
		return
	}

	// insert blob
	// https://code.google.com/p/go-wiki/wiki/SliceTricks
	*m = append(*m, nil)
	copy((*m)[pos+1:], (*m)[pos:])
	(*m)[pos] = blob
}

func (m *StorageMap) Merge(sm *StorageMap) {
	for _, blob := range *sm {
		m.Insert(blob)
	}
}
