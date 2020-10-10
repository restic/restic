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
	// Always add an empty final index, such that MergeFinalIndexes can merge into this.
	// Note that removing this index could lead to a race condition in the rare
	// sitation that only two indexes exist which are saved and merged concurrently.
	idx := []*Index{NewIndex()}
	idx[0].Finalize()
	return &MasterIndex{idx: idx, pendingBlobs: restic.NewBlobSet()}
}

// Lookup queries all known Indexes for the ID and returns all matches.
func (mi *MasterIndex) Lookup(id restic.ID, tpe restic.BlobType) (blobs []restic.PackedBlob) {
	mi.idxMutex.RLock()
	defer mi.idxMutex.RUnlock()

	for _, idx := range mi.idx {
		blobs = idx.Lookup(id, tpe, blobs)
	}

	return blobs
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

// Packs returns all packs that are covered by the index.
func (mi *MasterIndex) Packs() restic.IDSet {
	mi.idxMutex.RLock()
	defer mi.idxMutex.RUnlock()

	packs := restic.NewIDSet()
	for _, idx := range mi.idx {
		packs.Merge(idx.Packs())
	}

	return packs
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

// StorePack remembers the id and pack in the index.
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

// MergeFinalIndexes merges all final indexes together.
// After calling, there will be only one big final index in MasterIndex
// containing all final index contents.
// Indexes that are not final are left untouched.
// This merging can only be called after all index files are loaded - as
// removing of superseded index contents is only possible for unmerged indexes.
func (mi *MasterIndex) MergeFinalIndexes() {
	mi.idxMutex.Lock()
	defer mi.idxMutex.Unlock()

	// The first index is always final and the one to merge into
	newIdx := mi.idx[:1]
	for i := 1; i < len(mi.idx); i++ {
		idx := mi.idx[i]
		// clear reference in masterindex as it may become stale
		mi.idx[i] = nil
		if !idx.Final() {
			newIdx = append(newIdx, idx)
		} else {
			mi.idx[0].merge(idx)
		}
	}
	mi.idx = newIdx
}

// Save saves all known indexes to index files, leaving out any
// packs whose ID is contained in packBlacklist. The new index contains the IDs
// of all known indexes in the "supersedes" field. The IDs are also returned in
// the IDSet obsolete
// After calling this function, you should remove the obsolete index files.
func (mi *MasterIndex) Save(ctx context.Context, repo restic.Repository, packBlacklist restic.IDSet, p *restic.Progress) (obsolete restic.IDSet, err error) {
	p.Start()
	defer p.Done()

	mi.idxMutex.Lock()
	defer mi.idxMutex.Unlock()

	debug.Log("start rebuilding index of %d indexes, pack blacklist: %v", len(mi.idx), packBlacklist)

	newIndex := NewIndex()
	obsolete = restic.NewIDSet()

	finalize := func() error {
		newIndex.Finalize()
		if _, err := SaveIndex(ctx, repo, newIndex); err != nil {
			return err
		}
		newIndex = NewIndex()
		return nil
	}

	for i, idx := range mi.idx {
		if idx.Final() {
			ids, err := idx.IDs()
			if err != nil {
				debug.Log("index %d does not have an ID: %v", err)
				return nil, err
			}

			debug.Log("adding index ids %v to supersedes field", ids)

			err = newIndex.AddToSupersedes(ids...)
			if err != nil {
				return nil, err
			}
			obsolete.Merge(restic.NewIDSet(ids...))
		} else {
			debug.Log("index %d isn't final, don't add to supersedes field", i)
		}

		debug.Log("adding index %d", i)

		for pbs := range idx.EachByPack(ctx, packBlacklist) {
			newIndex.StorePack(pbs.packID, pbs.blobs)
			p.Report(restic.Stat{Blobs: 1})
			if IndexFull(newIndex) {
				if err := finalize(); err != nil {
					return nil, err
				}
			}
		}
	}
	if err := finalize(); err != nil {
		return nil, err
	}

	return
}
