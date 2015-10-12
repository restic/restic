package repository

import (
	"fmt"
	"sync"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/debug"
	"github.com/restic/restic/pack"
)

// MasterIndex is a collection of indexes and IDs of chunks that are in the process of being saved.
type MasterIndex struct {
	idx      []*Index
	idxMutex sync.RWMutex

	inFlight struct {
		backend.IDSet
		sync.RWMutex
	}
}

// NewMasterIndex creates a new master index.
func NewMasterIndex() *MasterIndex {
	return &MasterIndex{
		inFlight: struct {
			backend.IDSet
			sync.RWMutex
		}{
			IDSet: backend.NewIDSet(),
		},
	}
}

// Lookup queries all known Indexes for the ID and returns the first match.
func (mi *MasterIndex) Lookup(id backend.ID) (packID *backend.ID, tpe pack.BlobType, offset, length uint, err error) {
	mi.idxMutex.RLock()
	defer mi.idxMutex.RUnlock()

	debug.Log("MasterIndex.Lookup", "looking up id %v", id.Str())

	for _, idx := range mi.idx {
		packID, tpe, offset, length, err = idx.Lookup(id)
		if err == nil {
			debug.Log("MasterIndex.Lookup",
				"found id %v in pack %v at offset %d with length %d",
				id.Str(), packID.Str(), offset, length)
			return
		}
	}

	debug.Log("MasterIndex.Lookup", "id %v not found in any index", id.Str())
	return nil, pack.Data, 0, 0, fmt.Errorf("id %v not found in any index", id)
}

// LookupSize queries all known Indexes for the ID and returns the first match.
func (mi *MasterIndex) LookupSize(id backend.ID) (uint, error) {
	mi.idxMutex.RLock()
	defer mi.idxMutex.RUnlock()

	for _, idx := range mi.idx {
		length, err := idx.LookupSize(id)
		if err == nil {
			return length, nil
		}
	}

	return 0, fmt.Errorf("id %v not found in any index", id)
}

// Has queries all known Indexes for the ID and returns the first match.
func (mi *MasterIndex) Has(id backend.ID) bool {
	mi.idxMutex.RLock()
	defer mi.idxMutex.RUnlock()

	for _, idx := range mi.idx {
		if idx.Has(id) {
			return true
		}
	}

	return false
}

// Count returns the number of blobs of type t in the index.
func (mi *MasterIndex) Count(t pack.BlobType) (n uint) {
	mi.idxMutex.RLock()
	defer mi.idxMutex.RUnlock()

	var sum uint
	for _, idx := range mi.idx {
		sum += idx.Count(t)
	}

	return sum
}

// Insert adds a new index to the MasterIndex.
func (mi *MasterIndex) Insert(idx *Index) {
	mi.idxMutex.Lock()
	defer mi.idxMutex.Unlock()

	mi.idx = append(mi.idx, idx)
}

// Remove deletes an index from the MasterIndex.
func (mi *MasterIndex) Remove(index *Index) {
	mi.idxMutex.Lock()
	defer mi.idxMutex.Unlock()

	for i, idx := range mi.idx {
		if idx == index {
			mi.idx = append(mi.idx[:i], mi.idx[i+1:]...)
			return
		}
	}
}

// Current returns an index that is not yet finalized, so that new entries can
// still be added. If all indexes are finalized, a new index is created and
// returned.
func (mi *MasterIndex) Current() *Index {
	mi.idxMutex.RLock()

	for _, idx := range mi.idx {
		if !idx.Final() {
			mi.idxMutex.RUnlock()
			return idx
		}
	}

	mi.idxMutex.RUnlock()
	mi.idxMutex.Lock()
	defer mi.idxMutex.Unlock()

	newIdx := NewIndex()
	mi.idx = append(mi.idx, newIdx)

	return newIdx
}

// AddInFlight add the given IDs to the list of in-flight IDs.
func (mi *MasterIndex) AddInFlight(IDs ...backend.ID) {
	mi.inFlight.Lock()
	defer mi.inFlight.Unlock()

	ids := backend.IDs(IDs)

	debug.Log("MasterIndex.AddInFlight", "adding %v", ids)

	for _, id := range ids {
		mi.inFlight.Insert(id)
	}
}

// IsInFlight returns true iff the id is contained in the list of in-flight IDs.
func (mi *MasterIndex) IsInFlight(id backend.ID) bool {
	mi.inFlight.RLock()
	defer mi.inFlight.RUnlock()

	inFlight := mi.inFlight.Has(id)
	debug.Log("MasterIndex.IsInFlight", "testing whether %v is in flight: %v", id.Str(), inFlight)

	return inFlight
}

// RemoveFromInFlight deletes the given ID from the liste of in-flight IDs.
func (mi *MasterIndex) RemoveFromInFlight(id backend.ID) {
	mi.inFlight.Lock()
	defer mi.inFlight.Unlock()

	debug.Log("MasterIndex.RemoveFromInFlight", "removing %v from list of in flight blobs", id.Str())

	mi.inFlight.Delete(id)
}

// NotFinalIndexes returns all indexes that have not yet been saved.
func (mi *MasterIndex) NotFinalIndexes() []*Index {
	mi.idxMutex.Lock()
	defer mi.idxMutex.Unlock()

	var list []*Index

	for _, idx := range mi.idx {
		if !idx.Final() {
			list = append(list, idx)
		}
	}

	debug.Log("MasterIndex.NotFinalIndexes", "saving %d indexes", len(list))
	return list
}

// All returns all indexes.
func (mi *MasterIndex) All() []*Index {
	mi.idxMutex.Lock()
	defer mi.idxMutex.Unlock()

	return mi.idx
}
