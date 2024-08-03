package index

import (
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
}

// NewMasterIndex creates a new master index.
func NewMasterIndex() *MasterIndex {
	mi := &MasterIndex{pendingBlobs: restic.NewBlobSet()}
	mi.clear()
	return mi
}

func (mi *MasterIndex) clear() {
	// Always add an empty final index, such that MergeFinalIndexes can merge into this.
	mi.idx = []*Index{NewIndex()}
	mi.idx[0].Finalize()
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

// Each runs fn on all blobs known to the index. When the context is cancelled,
// the index iteration return immediately. This blocks any modification of the index.
func (mi *MasterIndex) Each(ctx context.Context, fn func(restic.PackedBlob)) error {
	mi.idxMutex.RLock()
	defer mi.idxMutex.RUnlock()

	for _, idx := range mi.idx {
		if err := idx.Each(ctx, fn); err != nil {
			return err
		}
	}
	return nil
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

func (mi *MasterIndex) Load(ctx context.Context, r restic.ListerLoaderUnpacked, p *progress.Counter, cb func(id restic.ID, idx *Index, oldFormat bool, err error) error) error {
	indexList, err := restic.MemorizeList(ctx, r, restic.IndexFile)
	if err != nil {
		return err
	}

	if p != nil {
		var numIndexFiles uint64
		err := indexList.List(ctx, restic.IndexFile, func(_ restic.ID, _ int64) error {
			numIndexFiles++
			return nil
		})
		if err != nil {
			return err
		}
		p.SetMax(numIndexFiles)
		defer p.Done()
	}

	err = ForAllIndexes(ctx, indexList, r, func(id restic.ID, idx *Index, oldFormat bool, err error) error {
		if p != nil {
			p.Add(1)
		}
		if cb != nil {
			err = cb(id, idx, oldFormat, err)
		}
		if err != nil {
			return err
		}
		// special case to allow check to ignore index loading errors
		if idx == nil {
			return nil
		}
		mi.Insert(idx)
		return nil
	})

	if err != nil {
		return err
	}

	return mi.MergeFinalIndexes()
}

type MasterIndexRewriteOpts struct {
	SaveProgress   *progress.Counter
	DeleteProgress func() *progress.Counter
	DeleteReport   func(id restic.ID, err error)
}

// Rewrite removes packs whose ID is in excludePacks from all known indexes.
// It also removes the rewritten index files and those listed in extraObsolete.
// If oldIndexes is not nil, then only the indexes in this set are processed.
// This is used by repair index to only rewrite and delete the old indexes.
//
// Must not be called concurrently to any other MasterIndex operation.
func (mi *MasterIndex) Rewrite(ctx context.Context, repo restic.Unpacked, excludePacks restic.IDSet, oldIndexes restic.IDSet, extraObsolete restic.IDs, opts MasterIndexRewriteOpts) error {
	for _, idx := range mi.idx {
		if !idx.Final() {
			panic("internal error - index must be saved before calling MasterIndex.Rewrite")
		}
	}

	var indexes restic.IDSet
	if oldIndexes != nil {
		// repair index adds new index entries for already existing pack files
		// only remove the old (possibly broken) entries by only processing old indexes
		indexes = oldIndexes
	} else {
		indexes = mi.IDs()
	}

	p := opts.SaveProgress
	p.SetMax(uint64(len(indexes)))

	// reset state which is not necessary for Rewrite and just consumes a lot of memory
	// the index state would be invalid after Rewrite completes anyways
	mi.clear()
	runtime.GC()

	// copy excludePacks to prevent unintended sideeffects
	excludePacks = excludePacks.Clone()
	debug.Log("start rebuilding index of %d indexes, excludePacks: %v", len(indexes), excludePacks)
	wg, wgCtx := errgroup.WithContext(ctx)

	idxCh := make(chan restic.ID)
	wg.Go(func() error {
		defer close(idxCh)
		for id := range indexes {
			select {
			case idxCh <- id:
			case <-wgCtx.Done():
				return wgCtx.Err()
			}
		}
		return nil
	})

	var rewriteWg sync.WaitGroup
	type rewriteTask struct {
		idx       *Index
		oldFormat bool
	}
	rewriteCh := make(chan rewriteTask)
	loader := func() error {
		defer rewriteWg.Done()
		for id := range idxCh {
			buf, err := repo.LoadUnpacked(wgCtx, restic.IndexFile, id)
			if err != nil {
				return fmt.Errorf("LoadUnpacked(%v): %w", id.Str(), err)
			}
			idx, oldFormat, err := DecodeIndex(buf, id)
			if err != nil {
				return err
			}

			select {
			case rewriteCh <- rewriteTask{idx, oldFormat}:
			case <-wgCtx.Done():
				return wgCtx.Err()
			}

		}
		return nil
	}
	// loading an index can take quite some time such that this is probably CPU-bound
	// the index files are probably already cached at this point
	loaderCount := runtime.GOMAXPROCS(0)
	// run workers on ch
	for i := 0; i < loaderCount; i++ {
		rewriteWg.Add(1)
		wg.Go(loader)
	}
	wg.Go(func() error {
		rewriteWg.Wait()
		close(rewriteCh)
		return nil
	})

	obsolete := restic.NewIDSet(extraObsolete...)
	saveCh := make(chan *Index)

	wg.Go(func() error {
		defer close(saveCh)
		newIndex := NewIndex()
		for task := range rewriteCh {
			// always rewrite indexes using the old format, that include a pack that must be removed or that are not full
			if !task.oldFormat && len(task.idx.Packs().Intersect(excludePacks)) == 0 && IndexFull(task.idx) {
				// make sure that each pack is only stored exactly once in the index
				excludePacks.Merge(task.idx.Packs())
				// index is already up to date
				p.Add(1)
				continue
			}

			ids, err := task.idx.IDs()
			if err != nil || len(ids) != 1 {
				panic("internal error, index has no ID")
			}
			obsolete.Merge(restic.NewIDSet(ids...))

			for pbs := range task.idx.EachByPack(wgCtx, excludePacks) {
				newIndex.StorePack(pbs.PackID, pbs.Blobs)
				if IndexFull(newIndex) {
					select {
					case saveCh <- newIndex:
					case <-wgCtx.Done():
						return wgCtx.Err()
					}
					newIndex = NewIndex()
				}
			}
			if wgCtx.Err() != nil {
				return wgCtx.Err()
			}
			// make sure that each pack is only stored exactly once in the index
			excludePacks.Merge(task.idx.Packs())
			p.Add(1)
		}

		select {
		case saveCh <- newIndex:
		case <-wgCtx.Done():
		}
		return nil
	})

	// a worker receives an index from ch, and saves the index
	worker := func() error {
		for idx := range saveCh {
			idx.Finalize()
			if len(idx.packs) == 0 {
				continue
			}
			if _, err := idx.SaveIndex(wgCtx, repo); err != nil {
				return err
			}
		}
		return nil
	}

	// encoding an index can take quite some time such that this can be CPU- or IO-bound
	// do not add repo.Connections() here as there are already the loader goroutines.
	workerCount := runtime.GOMAXPROCS(0)
	// run workers on ch
	for i := 0; i < workerCount; i++ {
		wg.Go(worker)
	}
	err := wg.Wait()
	p.Done()
	if err != nil {
		return fmt.Errorf("failed to rewrite indexes: %w", err)
	}

	p = nil
	if opts.DeleteProgress != nil {
		p = opts.DeleteProgress()
	}
	defer p.Done()
	return restic.ParallelRemove(ctx, repo, obsolete, restic.IndexFile, func(id restic.ID, err error) error {
		if opts.DeleteReport != nil {
			opts.DeleteReport(id, err)
		}
		return err
	}, p)
}

// SaveFallback saves all known indexes to index files, leaving out any
// packs whose ID is contained in packBlacklist from finalized indexes.
// It is only intended for use by prune with the UnsafeRecovery option.
//
// Must not be called concurrently to any other MasterIndex operation.
func (mi *MasterIndex) SaveFallback(ctx context.Context, repo restic.SaverRemoverUnpacked, excludePacks restic.IDSet, p *progress.Counter) error {
	p.SetMax(uint64(len(mi.Packs(excludePacks))))

	mi.idxMutex.Lock()
	defer mi.idxMutex.Unlock()

	debug.Log("start rebuilding index of %d indexes, excludePacks: %v", len(mi.idx), excludePacks)

	obsolete := restic.NewIDSet()
	wg, wgCtx := errgroup.WithContext(ctx)

	ch := make(chan *Index)
	wg.Go(func() error {
		defer close(ch)
		newIndex := NewIndex()
		for _, idx := range mi.idx {
			if idx.Final() {
				ids, err := idx.IDs()
				if err != nil {
					panic("internal error - finalized index without ID")
				}
				debug.Log("adding index ids %v to supersedes field", ids)
				obsolete.Merge(restic.NewIDSet(ids...))
			}

			for pbs := range idx.EachByPack(wgCtx, excludePacks) {
				newIndex.StorePack(pbs.PackID, pbs.Blobs)
				p.Add(1)
				if IndexFull(newIndex) {
					select {
					case ch <- newIndex:
					case <-wgCtx.Done():
						return wgCtx.Err()
					}
					newIndex = NewIndex()
				}
			}
			if wgCtx.Err() != nil {
				return wgCtx.Err()
			}
		}

		select {
		case ch <- newIndex:
		case <-wgCtx.Done():
		}
		return nil
	})

	// a worker receives an index from ch, and saves the index
	worker := func() error {
		for idx := range ch {
			idx.Finalize()
			if _, err := idx.SaveIndex(wgCtx, repo); err != nil {
				return err
			}
		}
		return nil
	}

	// keep concurrency bounded as we're on a fallback path
	workerCount := int(repo.Connections())
	// run workers on ch
	for i := 0; i < workerCount; i++ {
		wg.Go(worker)
	}
	err := wg.Wait()
	p.Done()
	// the index no longer matches to stored state
	mi.clear()

	return err
}

// saveIndex saves all indexes in the backend.
func (mi *MasterIndex) saveIndex(ctx context.Context, r restic.SaverUnpacked, indexes ...*Index) error {
	for i, idx := range indexes {
		debug.Log("Saving index %d", i)

		sid, err := idx.SaveIndex(ctx, r)
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
			packBlob := make(map[restic.ID][]restic.Blob)
			for pack := range packs {
				if pack[0]&0xf == i {
					packBlob[pack] = nil
				}
			}
			if len(packBlob) == 0 {
				continue
			}
			err := mi.Each(ctx, func(pb restic.PackedBlob) {
				if packs.Has(pb.PackID) && pb.PackID[0]&0xf == i {
					packBlob[pb.PackID] = append(packBlob[pb.PackID], pb.Blob)
				}
			})
			if err != nil {
				return
			}

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

// Only for use by AssociatedSet
func (mi *MasterIndex) blobIndex(h restic.BlobHandle) int {
	mi.idxMutex.RLock()
	defer mi.idxMutex.RUnlock()

	// other indexes are ignored as their ids can change when merged into the main index
	return mi.idx[0].BlobIndex(h)
}

// Only for use by AssociatedSet
func (mi *MasterIndex) stableLen(t restic.BlobType) uint {
	mi.idxMutex.RLock()
	defer mi.idxMutex.RUnlock()

	// other indexes are ignored as their ids can change when merged into the main index
	return mi.idx[0].Len(t)
}
