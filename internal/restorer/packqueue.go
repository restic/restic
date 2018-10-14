package restorer

import (
	"container/heap"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
)

// packQueue tracks remaining file contents restore work and decides what pack
// to download and files to write next.
//
// The packs in the queue can be in one of three states: waiting, ready and
// in-progress.
// Waiting packs are the packs that only have blobs from the "middle" of their
// corresponding files and therefore cannot be used until blobs from some other
// packs are written to the files first.
// In-progress packs are the packs that were removed from the queue by #nextPack
// and must be first returned to the queue before they are considered again.
// Ready packs are the packs can be immediately used to restore at least one
// file. Internally ready packs are kept in a heap and are ordered according
// to these criteria:
// - Packs with "head" blobs of in-progress files are considered first. The
//   idea is to complete restore of in-progress files before starting restore
//   of other files. This is both more intuitive and also reduces number of
//   open file handles needed during restore.
// - Packs with smallest cost are considered next. Pack cost is measured in
//   number of other packs required before all blobs in the pack can be used
//   and the pack can be removed from the pack cache.
//   For example, consisder a file that requires two blobs, blob1 from pack1
//   and blob2 from pack2. The cost of pack2 is 1, because blob2 cannot be
//   used before blob1 is available. The higher the cost, the longer the pack
//   must be cached locally to avoid redownload.
//
// Pack queue implementation is NOT thread safe. All pack queue methods must
// be called from single gorouting AND packInfo and fileInfo instances must
// be updated synchronously from the same gorouting.
type packQueue struct {
	idx filePackTraverser

	packs      map[restic.ID]*packInfo // waiting and ready packs
	inprogress map[*packInfo]struct{}  // inprogress packs

	heap *packHeap // heap of ready packs
}

func newPackQueue(idx filePackTraverser, files []*fileInfo, inprogress func(files map[*fileInfo]struct{}) bool) (*packQueue, error) {
	packs := make(map[restic.ID]*packInfo) // all packs

	// create packInfo from fileInfo
	for _, file := range files {
		err := idx.forEachFilePack(file, func(packIdx int, packID restic.ID, _ []restic.Blob) bool {
			pack, ok := packs[packID]
			if !ok {
				pack = &packInfo{
					id:    packID,
					index: -1,
					files: make(map[*fileInfo]struct{}),
				}
				packs[packID] = pack
			}
			pack.files[file] = struct{}{}
			pack.cost += packIdx

			return true // keep going
		})
		if err != nil {
			// repository index is messed up, can't do anything
			return nil, err
		}
	}

	// create packInfo heap
	pheap := &packHeap{inprogress: inprogress}
	headPacks := restic.NewIDSet()
	for _, file := range files {
		idx.forEachFilePack(file, func(packIdx int, packID restic.ID, _ []restic.Blob) bool {
			if !headPacks.Has(packID) {
				headPacks.Insert(packID)
				pack := packs[packID]
				pack.index = len(pheap.elements)
				pheap.elements = append(pheap.elements, pack)
			}
			return false // only first pack
		})
	}
	heap.Init(pheap)

	return &packQueue{idx: idx, packs: packs, heap: pheap, inprogress: make(map[*packInfo]struct{})}, nil
}

// isEmpty returns true if the queue is empty, i.e. there are no more packs to
// download and files to write to.
func (h *packQueue) isEmpty() bool {
	return len(h.packs) == 0 && len(h.inprogress) == 0
}

// nextPack returns next ready pack and corresponding files ready for download
// and processing. The returned pack and the files are marked as "in progress"
// internally and must  be first returned to the queue before they are
// considered by #nextPack again.
func (h *packQueue) nextPack() (*packInfo, []*fileInfo) {
	debug.Log("Ready packs %d, outstanding packs %d, inprogress packs %d", h.heap.Len(), len(h.packs), len(h.inprogress))

	if h.heap.Len() == 0 {
		return nil, nil
	}

	pack := heap.Pop(h.heap).(*packInfo)
	h.inprogress[pack] = struct{}{}
	debug.Log("Popped pack %s (%d files), heap size=%d", pack.id.Str(), len(pack.files), len(h.heap.elements))
	var files []*fileInfo
	for file := range pack.files {
		h.idx.forEachFilePack(file, func(packIdx int, packID restic.ID, packBlobs []restic.Blob) bool {
			debug.Log("Pack #%d %s (%d blobs) used by %s", packIdx, packID.Str(), len(packBlobs), file.location)
			if pack.id == packID {
				files = append(files, file)
			}
			return false // only interested in the fist pack here
		})
	}

	return pack, files
}

// requeuePack conditionally adds back to the queue pack previously returned by
// #nextPack.
// If the pack is needed to restore any incomplete files, adds the pack to the
// queue and adjusts order of all affected packs in the queue. Has no effect
// if the pack is not required to restore any files.
// Returns true if the pack was added to the queue, false otherwise.
func (h *packQueue) requeuePack(pack *packInfo, success []*fileInfo, failure []*fileInfo) bool {
	debug.Log("Requeue pack %s (%d/%d/%d files/success/failure)", pack.id.Str(), len(pack.files), len(success), len(failure))

	// maintain inprogress pack set
	delete(h.inprogress, pack)

	affectedPacks := make(map[*packInfo]struct{})
	affectedPacks[pack] = struct{}{} // this pack is alwats affected

	// apply download success/failure to the packs
	onFailure := func(file *fileInfo) {
		h.idx.forEachFilePack(file, func(packInx int, packID restic.ID, _ []restic.Blob) bool {
			pack := h.packs[packID]
			delete(pack.files, file)
			pack.cost -= packInx
			affectedPacks[pack] = struct{}{}
			return true // keep going
		})
	}
	for _, file := range failure {
		onFailure(file)
	}
	onSuccess := func(pack *packInfo, file *fileInfo) {
		remove := true
		h.idx.forEachFilePack(file, func(packIdx int, packID restic.ID, _ []restic.Blob) bool {
			if packID.Equal(pack.id) {
				// the pack has more blobs required by the file
				remove = false
			}
			otherPack := h.packs[packID]
			otherPack.cost--
			affectedPacks[otherPack] = struct{}{}
			return true // keep going
		})
		if remove {
			delete(pack.files, file)
		}
	}
	for _, file := range success {
		onSuccess(pack, file)
	}

	// drop/update affected packs
	isReady := func(affectedPack *packInfo) (ready bool) {
		for file := range affectedPack.files {
			h.idx.forEachFilePack(file, func(packIdx int, packID restic.ID, _ []restic.Blob) bool {
				if packID.Equal(affectedPack.id) {
					ready = true
				}
				return false // only file's first pack matters
			})
			if ready {
				break
			}
		}
		return ready
	}
	for affectedPack := range affectedPacks {
		if _, inprogress := h.inprogress[affectedPack]; !inprogress {
			if len(affectedPack.files) == 0 {
				// drop the pack if it isn't inprogress and has no files that need it
				if affectedPack.index >= 0 {
					// This can't happen unless there is a bug elsewhere:
					// - "current" pack isn't in the heap, hence its index must be < 0
					// - "other" packs can't be ready (i.e. in heap) unless they have other files
					//   in which case len(affectedPack.files) must be > 0
					debug.Log("corrupted ready heap: removed unexpected ready pack %s", affectedPack.id.Str())
					heap.Remove(h.heap, affectedPack.index)
				}
				delete(h.packs, affectedPack.id)
			} else {
				ready := isReady(affectedPack)
				switch {
				case ready && affectedPack.index < 0:
					heap.Push(h.heap, affectedPack)
				case ready && affectedPack.index >= 0:
					heap.Fix(h.heap, affectedPack.index)
				case !ready && affectedPack.index >= 0:
					// This can't happen unless there is a bug elsewhere:
					// - "current" pack isn't in the heap, hence its index must be < 0
					// - "other" packs can't have same head blobs as the "current" pack,
					//   hence "other" packs can't change their readiness
					debug.Log("corrupted ready heap: removed unexpected waiting pack %s", affectedPack.id.Str())
					heap.Remove(h.heap, affectedPack.index)
				case !ready && affectedPack.index < 0:
					// do nothing
				}
			}
		}
	}

	return len(pack.files) > 0
}
