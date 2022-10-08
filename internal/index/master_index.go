package index

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/progress"
	"golang.org/x/sync/errgroup"
)

// MasterIndex is a collection of indexes and IDs of chunks that are in the process of being saved.
type MasterIndex struct {
	idx          []*Index
	pendingBlobs restic.BlobSet
	idxMutex     sync.RWMutex
	compress     bool
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

func (mi *MasterIndex) MarkCompressed() {
	mi.compress = true
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
func (mi *MasterIndex) AddPending(bh restic.BlobHandle) bool {

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

// IDs returns the IDs of all indexes contained in the index.
func (mi *MasterIndex) IDs() restic.IDSet {
	mi.idxMutex.RLock()
	defer mi.idxMutex.RUnlock()

	ids := restic.NewIDSet()
	for _, idx := range mi.idx {
		if !idx.Final() {
			continue
		}
		indexIDs, err := idx.IDs()
		if err != nil {
			debug.Log("not using index, ID() returned error %v", err)
			continue
		}
		for _, id := range indexIDs {
			ids.Insert(id)
		}
	}
	return ids
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
		if idx.final && len(packBlacklist) > 0 {
			idxPacks = idxPacks.Sub(packBlacklist)
		}
		packs.Merge(idxPacks)
	}

	return packs
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

// finalizeNotFinalIndexes finalizes all indexes that
// have not yet been saved and returns that list
func (mi *MasterIndex) finalizeNotFinalIndexes() []*Index {
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

// finalizeFullIndexes finalizes all indexes that are full and returns that list.
func (mi *MasterIndex) finalizeFullIndexes() []*Index {
	mi.idxMutex.Lock()
	defer mi.idxMutex.Unlock()

	var list []*Index

	debug.Log("checking %d indexes", len(mi.idx))
	for _, idx := range mi.idx {
		if idx.Final() {
			continue
		}

		if IndexFull(idx, mi.compress) {
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

// Each runs fn on all blobs known to the index. When the context is cancelled,
// the index iteration return immediately. This blocks any modification of the index.
func (mi *MasterIndex) Each(ctx context.Context, fn func(restic.PackedBlob)) {
	mi.idxMutex.RLock()
	defer mi.idxMutex.RUnlock()

	for _, idx := range mi.idx {
		idx.Each(ctx, fn)
	}
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
		// do not merge indexes that have no id set
		ids, _ := idx.IDs()
		if !idx.Final() || len(ids) == 0 {
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

// Save saves all known indexes to index files, leaving out any
// packs whose ID is contained in packBlacklist from finalized indexes.
// The new index contains the IDs of all known indexes in the "supersedes"
// field. The IDs are also returned in the IDSet obsolete.
// After calling this function, you should remove the obsolete index files.
func (mi *MasterIndex) Save(ctx context.Context, repo restic.SaverUnpacked, packBlacklist restic.IDSet, extraObsolete restic.IDs, p *progress.Counter) (obsolete restic.IDSet, err error) {
	p.SetMax(uint64(len(mi.Packs(packBlacklist))))

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
				newIndex.StorePack(pbs.PackID, pbs.Blobs)
				p.Add(1)
				if IndexFull(newIndex, mi.compress) {
					select {
					case ch <- newIndex:
					case <-ctx.Done():
						return ctx.Err()
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

	// encoding an index can take quite some time such that this can be both CPU- or IO-bound
	workerCount := int(repo.Connections()) + runtime.GOMAXPROCS(0)
	// run workers on ch
	for i := 0; i < workerCount; i++ {
		wg.Go(worker)
	}
	err = wg.Wait()

	return obsolete, err
}

// SaveIndex saves an index in the repository.
func SaveIndex(ctx context.Context, repo restic.SaverUnpacked, index *Index) (restic.ID, error) {
	buf := bytes.NewBuffer(nil)

	err := index.Encode(buf)
	if err != nil {
		return restic.ID{}, err
	}

	id, err := repo.SaveUnpacked(ctx, restic.IndexFile, buf.Bytes())
	ierr := index.SetID(id)
	if ierr != nil {
		// logic bug
		panic(ierr)
	}
	return id, err
}

// saveIndex saves all indexes in the backend.
func (mi *MasterIndex) saveIndex(ctx context.Context, r restic.SaverUnpacked, indexes ...*Index) error {
	for i, idx := range indexes {
		debug.Log("Saving index %d", i)

		sid, err := SaveIndex(ctx, r, idx)
		if err != nil {
			return err
		}

		debug.Log("Saved index %d as %v", i, sid)
	}

	return mi.MergeFinalIndexes()
}

// SaveIndex saves all new indexes in the backend.
func (mi *MasterIndex) SaveIndex(ctx context.Context, r restic.SaverUnpacked) error {
	return mi.saveIndex(ctx, r, mi.finalizeNotFinalIndexes()...)
}

// SaveFullIndex saves all full indexes in the backend.
func (mi *MasterIndex) SaveFullIndex(ctx context.Context, r restic.SaverUnpacked) error {
	return mi.saveIndex(ctx, r, mi.finalizeFullIndexes()...)
}

// ListPacks returns the blobs of the specified pack files grouped by pack file.
func (mi *MasterIndex) ListPacks(ctx context.Context, packs restic.IDSet) <-chan restic.PackBlobs {
	out := make(chan restic.PackBlobs)
	go func() {
		defer close(out)
		// only resort a part of the index to keep the memory overhead bounded
		for i := byte(0); i < 16; i++ {
			if ctx.Err() != nil {
				return
			}

			packBlob := make(map[restic.ID][]restic.Blob)
			for pack := range packs {
				if pack[0]&0xf == i {
					packBlob[pack] = nil
				}
			}
			if len(packBlob) == 0 {
				continue
			}
			mi.Each(ctx, func(pb restic.PackedBlob) {
				if packs.Has(pb.PackID) && pb.PackID[0]&0xf == i {
					packBlob[pb.PackID] = append(packBlob[pb.PackID], pb.Blob)
				}
			})

			// pass on packs
			for packID, pbs := range packBlob {
				// allow GC
				packBlob[packID] = nil
				select {
				case out <- restic.PackBlobs{PackID: packID, Blobs: pbs}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out
}
