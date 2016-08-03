package repository

import (
	"fmt"
	"sync"

	"restic/backend"
	"restic/debug"
	"restic/pack"
)

// MasterIndex is a collection of indexes and IDs of chunks that are in the process of being saved.
type MasterIndex struct {
	idx      []*Index
	idxMutex sync.RWMutex
}

// NewMasterIndex creates a new master index.
func NewMasterIndex() *MasterIndex {
	return &MasterIndex{}
}

// Lookup queries all known Indexes for the ID and returns the first match.
func (mi *MasterIndex) Lookup(id backend.ID, tpe pack.BlobType) (blobs []PackedBlob, err error) {
	mi.idxMutex.RLock()
	defer mi.idxMutex.RUnlock()

	debug.Log("MasterIndex.Lookup", "looking up id %v, tpe %v", id.Str(), tpe)

	for _, idx := range mi.idx {
		blobs, err = idx.Lookup(id, tpe)
		if err == nil {
			debug.Log("MasterIndex.Lookup",
				"found id %v: %v", id.Str(), blobs)
			return
		}
	}

	debug.Log("MasterIndex.Lookup", "id %v not found in any index", id.Str())
	return nil, fmt.Errorf("id %v not found in any index", id)
}

// LookupSize queries all known Indexes for the ID and returns the first match.
func (mi *MasterIndex) LookupSize(id backend.ID, tpe pack.BlobType) (uint, error) {
	mi.idxMutex.RLock()
	defer mi.idxMutex.RUnlock()

	for _, idx := range mi.idx {
		length, err := idx.LookupSize(id, tpe)
		if err == nil {
			return length, nil
		}
	}

	return 0, fmt.Errorf("id %v not found in any index", id)
}

// ListPack returns the list of blobs in a pack. The first matching index is
// returned, or nil if no index contains information about the pack id.
func (mi *MasterIndex) ListPack(id backend.ID) (list []PackedBlob) {
	mi.idxMutex.RLock()
	defer mi.idxMutex.RUnlock()

	for _, idx := range mi.idx {
		list := idx.ListPack(id)
		if len(list) > 0 {
			return list
		}
	}

	return nil
}

// Has queries all known Indexes for the ID and returns the first match.
func (mi *MasterIndex) Has(id backend.ID, tpe pack.BlobType) bool {
	mi.idxMutex.RLock()
	defer mi.idxMutex.RUnlock()

	for _, idx := range mi.idx {
		if idx.Has(id, tpe) {
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

	debug.Log("MasterIndex.NotFinalIndexes", "return %d indexes", len(list))
	return list
}

// FullIndexes returns all indexes that are full.
func (mi *MasterIndex) FullIndexes() []*Index {
	mi.idxMutex.Lock()
	defer mi.idxMutex.Unlock()

	var list []*Index

	debug.Log("MasterIndex.FullIndexes", "checking %d indexes", len(mi.idx))
	for _, idx := range mi.idx {
		if idx.Final() {
			debug.Log("MasterIndex.FullIndexes", "index %p is final", idx)
			continue
		}

		if IndexFull(idx) {
			debug.Log("MasterIndex.FullIndexes", "index %p is full", idx)
			list = append(list, idx)
		} else {
			debug.Log("MasterIndex.FullIndexes", "index %p not full", idx)
		}
	}

	debug.Log("MasterIndex.FullIndexes", "return %d indexes", len(list))
	return list
}

// All returns all indexes.
func (mi *MasterIndex) All() []*Index {
	mi.idxMutex.Lock()
	defer mi.idxMutex.Unlock()

	return mi.idx
}

// RebuildIndex combines all known indexes to a new index, leaving out any
// packs whose ID is contained in packBlacklist. The new index contains the IDs
// of all known indexes in the "supersedes" field.
func (mi *MasterIndex) RebuildIndex(packBlacklist backend.IDSet) (*Index, error) {
	mi.idxMutex.Lock()
	defer mi.idxMutex.Unlock()

	debug.Log("MasterIndex.RebuildIndex", "start rebuilding index of %d indexes, pack blacklist: %v", len(mi.idx), packBlacklist)

	newIndex := NewIndex()
	done := make(chan struct{})
	defer close(done)

	for i, idx := range mi.idx {
		debug.Log("MasterIndex.RebuildIndex", "adding index %d", i)

		for pb := range idx.Each(done) {
			if packBlacklist.Has(pb.PackID) {
				continue
			}

			newIndex.Store(pb)
		}

		if !idx.Final() {
			debug.Log("MasterIndex.RebuildIndex", "index %d isn't final, don't add to supersedes field", i)
			continue
		}

		id, err := idx.ID()
		if err != nil {
			debug.Log("MasterIndex.RebuildIndex", "index %d does not have an ID: %v", err)
			return nil, err
		}

		debug.Log("MasterIndex.RebuildIndex", "adding index id %v to supersedes field", id.Str())

		err = newIndex.AddToSupersedes(id)
		if err != nil {
			return nil, err
		}
	}

	return newIndex, nil
}
