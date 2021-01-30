package repository

import (
	"context"
	"fmt"
	"sync"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/pack"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/progress"
	"golang.org/x/sync/errgroup"
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
func (mi *MasterIndex) Lookup(bh restic.BlobHandle) (pbs []restic.PackedBlob) {
	mi.idxMutex.RLock()
	defer mi.idxMutex.RUnlock()

	for _, idx := range mi.idx {
		pbs = idx.Lookup(bh, pbs)
	}

	return pbs
}

// LookupSize queries all known Indexes for the ID and returns the first match.
func (mi *MasterIndex) LookupSize(bh restic.BlobHandle) (uint, bool) {
	mi.idxMutex.RLock()
	defer mi.idxMutex.RUnlock()

	for _, idx := range mi.idx {
		if size, found := idx.LookupSize(bh); found {
			return size, found
		}
	}

	return 0, false
}

// AddPending adds a given blob to list of pending Blobs
// Before doing so it checks if this blob is already known.
// Returns true if adding was successful and false if the blob
// was already known
func (mi *MasterIndex) addPending(bh restic.BlobHandle) bool {

	mi.idxMutex.Lock()
	defer mi.idxMutex.Unlock()

	// Check if blob is pending or in index
	if mi.pendingBlobs.Has(bh) {
		return false
	}

	for _, idx := range mi.idx {
		if idx.Has(bh) {
			return false
		}
	}

	// really not known -> insert
	mi.pendingBlobs.Insert(bh)
	return true
}

// Has queries all known Indexes for the ID and returns the first match.
// Also returns true if the ID is pending.
func (mi *MasterIndex) Has(bh restic.BlobHandle) bool {
	mi.idxMutex.RLock()
	defer mi.idxMutex.RUnlock()

	// also return true if blob is pending
	if mi.pendingBlobs.Has(bh) {
		return true
	}

	for _, idx := range mi.idx {
		if idx.Has(bh) {
			return true
		}
	}

	return false
}

// Packs returns all packs that are covered by the index.
// If packBlacklist is given, those packs are only contained in the
// resulting IDSet if they are contained in a non-final (newly written) index.
func (mi *MasterIndex) Packs(packBlacklist restic.IDSet) restic.IDSet {
	mi.idxMutex.RLock()
	defer mi.idxMutex.RUnlock()

	packs := restic.NewIDSet()
	for _, idx := range mi.idx {
		idxPacks := idx.Packs()
		if idx.final {
			idxPacks = idxPacks.Sub(packBlacklist)
		}
		packs.Merge(idxPacks)
	}

	return packs
}

// PackSize returns the size of all packs computed by index information.
// If onlyHdr is set to true, only the size of the header is returned
// Note that this function only gives correct sizes, if there are no
// duplicates in the index.
func (mi *MasterIndex) PackSize(ctx context.Context, onlyHdr bool) map[restic.ID]int64 {
	packSize := make(map[restic.ID]int64)

	for blob := range mi.Each(ctx) {
		size, ok := packSize[blob.PackID]
		if !ok {
			size = pack.HeaderSize
		}
		if !onlyHdr {
			size += int64(blob.Length)
		}
		packSize[blob.PackID] = size + int64(pack.EntrySize)
	}

	return packSize
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
func (mi *MasterIndex) MergeFinalIndexes() error {
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
			err := mi.idx[0].merge(idx)
			if err != nil {
				return fmt.Errorf("MergeFinalIndexes: %w", err)
			}
		}
	}
	mi.idx = newIdx

	return nil
}

const saveIndexParallelism = 4

// Save saves all known indexes to index files, leaving out any
// packs whose ID is contained in packBlacklist from finalized indexes.
// The new index contains the IDs of all known indexes in the "supersedes"
// field. The IDs are also returned in the IDSet obsolete.
// After calling this function, you should remove the obsolete index files.
func (mi *MasterIndex) Save(ctx context.Context, repo restic.Repository, packBlacklist restic.IDSet, extraObsolete restic.IDs, p *progress.Counter) (obsolete restic.IDSet, err error) {
	mi.idxMutex.Lock()
	defer mi.idxMutex.Unlock()

	debug.Log("start rebuilding index of %d indexes, pack blacklist: %v", len(mi.idx), packBlacklist)

	newIndex := NewIndex()
	obsolete = restic.NewIDSet()

	// track spawned goroutines using wg, create a new context which is
	// cancelled as soon as an error occurs.
	wg, ctx := errgroup.WithContext(ctx)

	ch := make(chan *Index)

	wg.Go(func() error {
		defer close(ch)
		for i, idx := range mi.idx {
			if idx.Final() {
				ids, err := idx.IDs()
				if err != nil {
					debug.Log("index %d does not have an ID: %v", err)
					return err
				}

				debug.Log("adding index ids %v to supersedes field", ids)

				err = newIndex.AddToSupersedes(ids...)
				if err != nil {
					return err
				}
				obsolete.Merge(restic.NewIDSet(ids...))
			} else {
				debug.Log("index %d isn't final, don't add to supersedes field", i)
			}

			debug.Log("adding index %d", i)

			for pbs := range idx.EachByPack(ctx, packBlacklist) {
				newIndex.StorePack(pbs.packID, pbs.blobs)
				p.Add(1)
				if IndexFull(newIndex) {
					select {
					case ch <- newIndex:
					case <-ctx.Done():
						return nil
					}
					newIndex = NewIndex()
				}
			}
		}

		err = newIndex.AddToSupersedes(extraObsolete...)
		if err != nil {
			return err
		}
		obsolete.Merge(restic.NewIDSet(extraObsolete...))

		select {
		case ch <- newIndex:
		case <-ctx.Done():
		}
		return nil
	})

	// a worker receives an index from ch, and saves the index
	worker := func() error {
		for idx := range ch {
			idx.Finalize()
			if _, err := SaveIndex(ctx, repo, idx); err != nil {
				return err
			}
		}
		return nil
	}

	// run workers on ch
	wg.Go(func() error {
		return RunWorkers(saveIndexParallelism, worker)
	})

	err = wg.Wait()

	return obsolete, err
}
