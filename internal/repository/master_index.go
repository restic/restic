package repository

import (
	"context"
	"sync"

	"github.com/restic/restic/internal/restic"

	"github.com/restic/restic/internal/debug"
)

// MasterIndex is a collection of indexes and IDs of chunks that are in the process of being saved.
type MasterIndex struct {
	idx          []*Index
	pendingBlobs restic.BlobSet
	idxMutex     sync.RWMutex
}

// NewMasterIndex creates a new master index.
func NewMasterIndex() *MasterIndex {
	return &MasterIndex{pendingBlobs: restic.NewBlobSet()}
}

// Lookup queries all known Indexes for the ID and returns the first match.
func (mi *MasterIndex) Lookup(id restic.ID, tpe restic.BlobType) (blobs []restic.PackedBlob, found bool) {
	mi.idxMutex.RLock()
	defer mi.idxMutex.RUnlock()

	for _, idx := range mi.idx {
		blobs, found = idx.Lookup(id, tpe)
		if found {
			return
		}
	}

	return nil, false
}

// LookupSize queries all known Indexes for the ID and returns the first match.
func (mi *MasterIndex) LookupSize(id restic.ID, tpe restic.BlobType) (uint, bool) {
	mi.idxMutex.RLock()
	defer mi.idxMutex.RUnlock()

	for _, idx := range mi.idx {
		if size, found := idx.LookupSize(id, tpe); found {
			return size, found
		}
	}

	return 0, false
}

// ListPack returns the list of blobs in a pack. The first matching index is
// returned, or nil if no index contains information about the pack id.
func (mi *MasterIndex) ListPack(id restic.ID) (list []restic.PackedBlob) {
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

// AddPending adds a given blob to list of pending Blobs
// Before doing so it checks if this blob is already known.
// Returns true if adding was successful and false if the blob
// was already known
func (mi *MasterIndex) addPending(id restic.ID, tpe restic.BlobType) bool {

	mi.idxMutex.Lock()
	defer mi.idxMutex.Unlock()

	// Check if blob is pending or in index
	if mi.pendingBlobs.Has(restic.BlobHandle{ID: id, Type: tpe}) {
		return false
	}

	for _, idx := range mi.idx {
		if idx.Has(id, tpe) {
			return false
		}
	}

	// really not known -> insert
	mi.pendingBlobs.Insert(restic.BlobHandle{ID: id, Type: tpe})
	return true
}

// Has queries all known Indexes for the ID and returns the first match.
// Also returns true if the ID is pending.
func (mi *MasterIndex) Has(id restic.ID, tpe restic.BlobType) bool {
	mi.idxMutex.RLock()
	defer mi.idxMutex.RUnlock()

	// also return true if blob is pending
	if mi.pendingBlobs.Has(restic.BlobHandle{ID: id, Type: tpe}) {
		return true
	}

	for _, idx := range mi.idx {
		if idx.Has(id, tpe) {
			return true
		}
	}

	return false
}

// Count returns the number of blobs of type t in the index.
func (mi *MasterIndex) Count(t restic.BlobType) (n uint) {
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

// Store remembers the id and pack in the index.
func (mi *MasterIndex) StorePack(id restic.ID, blobs []restic.Blob) {
	mi.idxMutex.Lock()
	defer mi.idxMutex.Unlock()

	// delete blobs from pending
	for _, blob := range blobs {
		mi.pendingBlobs.Delete(restic.BlobHandle{Type: blob.Type, ID: blob.ID})
	}

	for _, idx := range mi.idx {
		if !idx.Final() {
			idx.StorePack(id, blobs)
			return
		}
	}

	newIdx := NewIndex()
	newIdx.StorePack(id, blobs)
	mi.idx = append(mi.idx, newIdx)
}

// FinalizeNotFinalIndexes finalizes all indexes that
// have not yet been saved and returns that list
func (mi *MasterIndex) FinalizeNotFinalIndexes() []*Index {
	mi.idxMutex.Lock()
	defer mi.idxMutex.Unlock()

	var list []*Index

	for _, idx := range mi.idx {
		if !idx.Final() {
			idx.Finalize()
			list = append(list, idx)
		}
	}

	debug.Log("return %d indexes", len(list))
	return list
}

// FinalizeFullIndexes finalizes all indexes that are full and returns that list.
func (mi *MasterIndex) FinalizeFullIndexes() []*Index {
	mi.idxMutex.Lock()
	defer mi.idxMutex.Unlock()

	var list []*Index

	debug.Log("checking %d indexes", len(mi.idx))
	for _, idx := range mi.idx {
		if idx.Final() {
			debug.Log("index %p is final", idx)
			continue
		}

		if IndexFull(idx) {
			debug.Log("index %p is full", idx)
			idx.Finalize()
			list = append(list, idx)
		} else {
			debug.Log("index %p not full", idx)
		}
	}

	debug.Log("return %d indexes", len(list))
	return list
}

// All returns all indexes.
func (mi *MasterIndex) All() []*Index {
	mi.idxMutex.Lock()
	defer mi.idxMutex.Unlock()

	return mi.idx
}

// Each returns a channel that yields all blobs known to the index. When the
// context is cancelled, the background goroutine terminates. This blocks any
// modification of the index.
func (mi *MasterIndex) Each(ctx context.Context) <-chan restic.PackedBlob {
	mi.idxMutex.RLock()

	ch := make(chan restic.PackedBlob)

	go func() {
		defer mi.idxMutex.RUnlock()
		defer func() {
			close(ch)
		}()

		for _, idx := range mi.idx {
			idxCh := idx.Each(ctx)
			for pb := range idxCh {
				select {
				case <-ctx.Done():
					return
				case ch <- pb:
				}
			}
		}
	}()

	return ch
}

// RebuildIndex combines all known indexes to a new index, leaving out any
// packs whose ID is contained in packBlacklist. The new index contains the IDs
// of all known indexes in the "supersedes" field.
func (mi *MasterIndex) RebuildIndex(packBlacklist restic.IDSet) (*Index, error) {
	mi.idxMutex.Lock()
	defer mi.idxMutex.Unlock()

	debug.Log("start rebuilding index of %d indexes, pack blacklist: %v", len(mi.idx), packBlacklist)

	newIndex := NewIndex()

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	for i, idx := range mi.idx {
		debug.Log("adding index %d", i)

		for pb := range idx.Each(ctx) {
			if packBlacklist.Has(pb.PackID) {
				continue
			}

			newIndex.Store(pb)
		}

		if !idx.Final() {
			debug.Log("index %d isn't final, don't add to supersedes field", i)
			continue
		}

		id, err := idx.ID()
		if err != nil {
			debug.Log("index %d does not have an ID: %v", err)
			return nil, err
		}

		debug.Log("adding index id %v to supersedes field", id)

		err = newIndex.AddToSupersedes(id)
		if err != nil {
			return nil, err
		}
	}

	return newIndex, nil
}
