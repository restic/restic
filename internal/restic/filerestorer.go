package restic

import (
	"container/heap"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"sync"

	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"

	"github.com/hashicorp/golang-lru"
)

// TODO if a blob is corrupt, there may be good blob copies in other packs
// TODO evaluate if it makes sense to split download and processing workers
// TODO evaluate if it makes sense to cache open output files
// TODO hardlink support
// TODO evaluate memory footprint for larger repositories, say 10M packs/10M files
// TODO unexport everything
// TODO avoid decrypting the same blob multiple times

// TODO review cache is actually working, add unit tests
// TODO actually cap pack cache capacity
// TODO close cached pack cache entries

// the goal of this restorer is to minimize number of remote repository requests
// while maintaining hard limits on local filesystem cache size and number of open files

// pack cost is measured in number of other packs required before all blobs in the pack can be used
// and the pack can be removed from the cache
// for example, consisder a file that requires two blobs, blob1 from pack1 and blob2 from pack2
// the cost of pack2 is 1, because blob2 cannot be used before blob1 is available
// the higher the cost, the longer the pack must be cached locally to avoid redownload

///////////////////////////////////////////////////////////////////////////////
// pack cache
///////////////////////////////////////////////////////////////////////////////

// transient bound cache of downloaded pack files
type packCache struct {
	// guards access to cache internal data structures
	lock sync.Mutex

	// cache capacity
	capacity          int64
	reservedCapacity  int64
	allocatedCapacity int64

	// pack records currently being used by active restore goroutings
	reservedPacks map[ID]packCacheRecord

	// unused allocated packs, can be deleted if necessary
	cachedPacks map[ID]packCacheRecord
}

type packCacheRecord struct {
	id             ID    // cached pack id
	offset, length int64 // cached pack byte range

	data      *os.File
	populated bool
}

func newPackCache() *packCache {
	return &packCache{
		capacity:      50 * 1024 * 1024,
		reservedPacks: make(map[ID]packCacheRecord),
		cachedPacks:   make(map[ID]packCacheRecord),
	}
}

// allocates cache record for the specified pack byte range
// returns existing record if it is already available in the cache
func (c *packCache) allocate(packID ID, offset int64, length int64) (packCacheRecord, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if offset < 0 || length < 0 {
		return packCacheRecord{}, errors.Errorf("illegal pack cache allocation range %s {offset: %d, length: %d}", packID.Str(), offset, length)
	}

	if c.reservedCapacity+length > c.capacity {
		return packCacheRecord{}, errors.Errorf("not enough cache capacity: requested %d, available %d", length, c.capacity-c.reservedCapacity)
	}

	if _, ok := c.reservedPacks[packID]; ok {
		return packCacheRecord{}, errors.Errorf("pack is already reserved %s", packID.Str())
	}

	// the pack is available in the cache but currently unused
	if pack, ok := c.cachedPacks[packID]; ok {
		// check if cached pack includes requested byte range
		// the range can shrink, but it never grows bigger unless there is a bug elsewhere
		if pack.offset > offset || (pack.offset+pack.length) < (offset+length) {
			return packCacheRecord{}, errors.Errorf("cached range %d-%d is smaller than requested range %d-%d for pack %s", pack.offset, pack.offset+pack.length, length, offset+length, packID.Str())
		}

		// move the pack to the used map
		delete(c.cachedPacks, packID)
		c.reservedPacks[packID] = pack
		c.reservedCapacity += pack.length

		return pack, nil
	}

	if c.allocatedCapacity+length > c.capacity {
		// TODO make room
	}

	file, err := ioutil.TempFile("", "restic-"+packID.Str())
	if err != nil {
		return packCacheRecord{}, err
	}

	pack := packCacheRecord{
		id:     packID,
		data:   file,
		offset: offset,
		length: length,
	}
	c.reservedPacks[pack.id] = pack
	c.allocatedCapacity += length
	c.reservedCapacity += length

	return pack, nil
}

// releases the pack record back to the cache
func (c *packCache) release(pack packCacheRecord) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	if _, ok := c.reservedPacks[pack.id]; !ok {
		return errors.Errorf("invalid pack release request")
	}

	delete(c.reservedPacks, pack.id)
	c.cachedPacks[pack.id] = pack
	c.reservedCapacity -= pack.length

	return nil
}

func (c *packCache) remove(packID ID) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	if _, ok := c.reservedPacks[packID]; ok {
		return errors.Errorf("invalid pack remove request, pack %s is reserved", packID.Str())
	}

	pack, ok := c.cachedPacks[packID]
	if !ok {
		return errors.Errorf("invalid pack remove request, pack %s is not cached", packID.Str())
	}

	delete(c.cachedPacks, pack.id)
	pack.data.Close()
	os.Remove(pack.data.Name())
	c.allocatedCapacity -= pack.length

	return nil
}

func (r *packCacheRecord) hasData() bool {
	return r.populated
}

func (r *packCacheRecord) setData(rd io.Reader) error {
	// reset the file in case of a download retry
	_, err := r.data.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}
	err = r.data.Truncate(0)
	if err != nil {
		return err
	}

	len, err := io.Copy(r.data, rd)
	if err != nil {
		return err
	}
	if len != r.length {
		return errors.Errorf("unexpected pack size: expected %d but got %d", r.length, len)
	}

	r.populated = true

	return nil
}

// ReadAt reads len(b) bytes from the File starting at byte offset off.
// It returns the number of bytes read and the error, if any.
func (r *packCacheRecord) readAt(b []byte, off int64) (n int, err error) {
	// TODO validate requested range is valid
	return r.data.ReadAt(b, off-r.offset)
}

///////////////////////////////////////////////////////////////////////////////
// fileInfo and packInfo
///////////////////////////////////////////////////////////////////////////////

// information about file being restored
type fileInfo struct {
	node *Node

	// XXX local file info, including hardlinks and files with the same content

	// tentatively, full path to the file
	path string

	// index of the next blob
	// TODO review concurrent access from multiple goroutings
	//      e.g., same blob is available from two concurrently downloaded packs
	blobIdx int
}

// information about a data pack required to restore one or more files
type packInfo struct {
	// the pack id
	id ID

	// set of files that use blobs from this pack
	// TODO consider using fileInfoSet
	files map[*fileInfo]struct{}

	// number of other packs that must be downloaded before all blobs in this pack can be used
	cost int

	// used by packHeap
	index int
}

///////////////////////////////////////////////////////////////////////////////
// filePackTraverser
///////////////////////////////////////////////////////////////////////////////

type filePackTraverser struct {
	idx Index
}

// iterates over all remaining packs of the file
func (t *filePackTraverser) forEachFilePack(file *fileInfo, fn func(packIdx int, packID ID, packBlobs []Blob) bool) error {
	if file.blobIdx >= len(file.node.Content) {
		return nil
	}

	getBlobPack := func(blobID ID) (PackedBlob, error) {
		packs, found := t.idx.Lookup(blobID, DataBlob)
		if !found {
			return PackedBlob{}, errors.Errorf("Unknown blob %s", blobID.String())
		}
		// TODO which pack to use if multiple packs have the blob?
		// MUST return the same pack for the same blob during the same execution
		return packs[0], nil
	}

	var prevPackID ID
	var prevPackBlobs []Blob
	packIdx := 0
	for _, blobID := range file.node.Content[file.blobIdx:] {
		packedBlob, err := getBlobPack(blobID)
		if err != nil {
			return err
		}
		if !prevPackID.IsNull() && prevPackID != packedBlob.PackID {
			if !fn(packIdx, prevPackID, prevPackBlobs) {
				return nil
			}
			packIdx++
		}
		if prevPackID != packedBlob.PackID {
			prevPackID = packedBlob.PackID
			prevPackBlobs = make([]Blob, 0)
		}
		prevPackBlobs = append(prevPackBlobs, packedBlob.Blob)
	}
	if len(prevPackBlobs) > 0 {
		fn(packIdx, prevPackID, prevPackBlobs)
	}
	return nil
}

///////////////////////////////////////////////////////////////////////////////
// fileInfoSet
///////////////////////////////////////////////////////////////////////////////

// provide put/remove/has operations on set of fileInfo references
type fileInfoSet map[*fileInfo]struct{}

func (fs fileInfoSet) has(file *fileInfo) bool {
	_, ok := fs[file]
	return ok
}

func (fs fileInfoSet) put(file *fileInfo) {
	fs[file] = struct{}{}
}

func (fs fileInfoSet) remove(file *fileInfo) {
	delete(fs, file)
}

///////////////////////////////////////////////////////////////////////////////
// packHeap
///////////////////////////////////////////////////////////////////////////////

// packHeap is a heap of packInfo references
// @see https://golang.org/pkg/container/heap/
// @see https://en.wikipedia.org/wiki/Heap_(data_structure)
type packHeap []*packInfo

func (pq packHeap) Len() int { return len(pq) }

func (pq packHeap) Less(a, b int) bool {
	packA, packB := pq[a], pq[b]

	if packA.cost < packB.cost {
		return true
	}
	ap := inprogress(packA.files)
	bp := inprogress(packB.files)
	if ap && !bp {
		return true
	}
	return false
}

// returns true if download of any of the files is in progress
func inprogress(files map[*fileInfo]struct{}) bool {
	for file := range files {
		if file.blobIdx > 0 && file.blobIdx < len(file.node.Content) {
			return true
		}
	}
	return false
}

func (pq packHeap) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *packHeap) Push(x interface{}) {
	n := len(*pq)
	item := x.(*packInfo)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *packHeap) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	item.index = -1 // for safety
	*pq = old[0 : n-1]
	return item
}

///////////////////////////////////////////////////////////////////////////////
// packQueue
///////////////////////////////////////////////////////////////////////////////

// packQueue keeps track of all outstanding pack downloads and decides
// what pack to download next.
type packQueue struct {
	idx filePackTraverser

	// all remaining packs, includes "ready" packs, does NOT include packs inprogress packs
	packs map[ID]*packInfo

	heap       packHeap    // heap of "ready" packs
	inprogress fileInfoSet // inprogress files
}

func newPackScheduler(idx filePackTraverser, files []*fileInfo) (*packQueue, error) {
	packs := make(map[ID]*packInfo) // all packs

	// create packInfo from fileInfo
	for _, file := range files {
		err := idx.forEachFilePack(file, func(packIdx int, packID ID, _ []Blob) bool {
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
	pheap := make(packHeap, 0)
	headPacks := NewIDSet()
	for _, file := range files {
		idx.forEachFilePack(file, func(packIdx int, packID ID, _ []Blob) bool {
			if !headPacks.Has(packID) {
				headPacks.Insert(packID)
				pack := packs[packID]
				pack.index = len(pheap)
				pheap = append(pheap, pack)
				debug.Log("Enqueued pack cost %s (%d files), heap size=%d, new pack index=%d", pack.id.Str(), len(pack.files), len(pheap), pack.index)
			}
			return false // only first pack
		})
	}
	heap.Init(&pheap)

	return &packQueue{idx: idx, packs: packs, heap: pheap, inprogress: make(fileInfoSet)}, nil
}

// hasNextPack returns true if there are more packs to download and process
// TODO consider isEmpty(), nextPack may still return nil even if the queue hasNextPack() returns true
func (h *packQueue) hasNextPack() bool {
	return h.heap.Len()+len(h.inprogress) > 0
}

func (h *packQueue) isPackDone(pack *packInfo) bool {
	_, ok := h.packs[pack.id]
	return !ok
}

// nextPack returns next pack and corresponding files ready for download and processing
// TODO map[*fileInfo]error return value is ugly, change it to []*fileInfo
func (h *packQueue) nextPack() (*packInfo, map[*fileInfo]error) {
	debug.Log("Ready packs %d, outstanding packs %d, inprogress files %d", h.heap.Len(), len(h.packs), len(h.inprogress))

	if h.heap.Len() == 0 {
		return nil, nil
	}

	pack := heap.Pop(&h.heap).(*packInfo)
	delete(h.packs, pack.id)
	debug.Log("Popped pack %s (%d files), heap size=%d", pack.id.Str(), len(pack.files), len(h.heap))
	files := make(map[*fileInfo]error)
	for file := range pack.files {
		if !h.inprogress.has(file) {
			h.idx.forEachFilePack(file, func(packIdx int, packID ID, packBlobs []Blob) bool {
				debug.Log("Pack #%d %s (%d blobs) used by %s", packIdx, packID.Str(), len(packBlobs), file.path)
				if pack.id == packID {
					files[file] = nil
					h.inprogress.put(file)
				}
				return false // only interested in the fist pack here
			})
		} else {
			debug.Log("Skipping inprogress %s", file.path)
		}
	}

	if len(files) == 0 {
		// all pack's files are in-progress
		// put the pack back to the queue, need to wait for feedback before can proceed
		h.packs[pack.id] = pack
		heap.Push(&h.heap, pack)
		return nil, nil
	}

	return pack, files
}

// returnPack puts the pack back to the queue
func (h *packQueue) returnPack(pack *packInfo, files map[*fileInfo]error) {
	debug.Log("Put back to the queue popped pack %s (%d files), heap size=%d", pack.id.Str(), len(pack.files), len(h.heap))
	h.packs[pack.id] = pack
	heap.Push(&h.heap, pack) // put the pack back into the heap
	for file := range files {
		h.inprogress.remove(file)
	}
}

func (h *packQueue) processFeedback(pack *packInfo, files map[*fileInfo]error) {
	debug.Log("Feedback pack %s (%d/%d files)", pack.id.Str(), len(files), len(pack.files))

	// maintain inprogress file set
	for file := range files {
		h.inprogress.remove(file)
	}

	// adjust costs of other affected packs
	for file := range files {
		h.idx.forEachFilePack(file, func(packIdx int, packID ID, _ []Blob) bool {
			if otherPack, ok := h.packs[packID]; ok {
				otherPack.cost--
				var verb string
				if otherPack.index >= 0 {
					verb = "Adjusted"
					heap.Fix(&h.heap, otherPack.index)
				} else if packIdx == 0 {
					verb = "Enqueued"
					heap.Push(&h.heap, otherPack)
				}
				debug.Log("%s pack cost %s (%d files), heap size=%d, new pack index=%d", verb, otherPack.id.Str(), len(otherPack.files), len(h.heap), otherPack.index)
			}

			return true // keep going
		})
	}

	// recalculate cost and pending files of the processed pack
	pack.cost = 0
	packFiles := make(map[*fileInfo]struct{}) // incomplete files still waiting for this pack
	for file := range pack.files {
		if files[file] != nil {
			continue
		}
		h.idx.forEachFilePack(file, func(packIdx int, packID ID, _ []Blob) bool {
			if packID.Equal(pack.id) {
				packFiles[file] = struct{}{}
				h.packs[pack.id] = pack
				if packIdx == 0 {
					if pack.index < 0 {
						heap.Push(&h.heap, pack)
					}
				} else {
					pack.cost += packIdx
					heap.Fix(&h.heap, pack.index)
				}
			}

			return true // keep going
		})
	}
	pack.files = packFiles
	if pack.index >= 0 {
		debug.Log("Requeued pack %s (%d files), heap size=%d, pack index=%d", pack.id.Str(), len(pack.files), len(h.heap), pack.index)
	}
}

///////////////////////////////////////////////////////////////////////////////
// FileRestorer
///////////////////////////////////////////////////////////////////////////////

// FileRestorer restores set of files
type FileRestorer struct {
	repo Repository
	idx  filePackTraverser

	cache *packCache

	files []*fileInfo

	writers *lru.Cache
}

// used to pass information among workers (wish golang channels allowed multivalues)
type processingInfo struct {
	pack  *packInfo
	files map[*fileInfo]error
}

func newFileRestorer(repo Repository) *FileRestorer {
	writers, _ := lru.NewWithEvict(32, func(key interface{}, value interface{}) {
		value.(io.Closer).Close()
	})
	return &FileRestorer{
		repo:    repo,
		idx:     filePackTraverser{idx: repo.Index()},
		cache:   newPackCache(),
		writers: writers,
	}
}

func (r *FileRestorer) RestoreFiles(ctx context.Context) error {
	queue, err := newPackScheduler(r.idx, r.files)
	if err != nil {
		return err
	}

	// workers
	downloadCh := make(chan processingInfo)
	feedbackCh := make(chan processingInfo)
	worker := func() {
		for {
			select {
			case <-ctx.Done():
				return
			case request := <-downloadCh:
				cachedPack, err := r.downloadPack(ctx, request.pack)
				if err == nil {
					r.processPack(ctx, request, cachedPack)
				} else {
					// mark all files as failed
					for file := range request.files {
						request.files[file] = err
					}
				}
				feedbackCh <- request
			}
		}
	}
	for i := 0; i < 8; i++ {
		go worker()
	}

	processFeedback := func(pack *packInfo, files map[*fileInfo]error) {
		// advance processed files blobIdx
		// must do it here to avoid race among worker and processing feedback threads
		for file := range files {
			r.idx.forEachFilePack(file, func(packIdx int, packID ID, packBlobs []Blob) bool {
				file.blobIdx += len(packBlobs)

				return false // only interesed in the first pack
			})
		}
		queue.processFeedback(pack, files)
		if queue.isPackDone(pack) {
			r.cache.remove(pack.id)
			debug.Log("Purged used up pack %s from pack cache", pack.id.Str())
		}
		for file, ferr := range files {
			if ferr != nil {
				fmt.Printf("Could not restore %s: %v\n", file.path, ferr)
				file.blobIdx = len(file.node.Content) // done with this file
				r.writers.Remove(file)
			} else if file.blobIdx >= len(file.node.Content) {
				fmt.Printf("Restored %s\n", file.path)
				r.writers.Remove(file)
			}
		}
	}

	// the main restore loop
	for queue.hasNextPack() {
		debug.Log("-----------------------------------")
		pack, files := queue.nextPack()
		if pack != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case downloadCh <- processingInfo{pack: pack, files: files}:
				debug.Log("Scheduled download pack %s (%d files)", pack.id.Str(), len(files))
			case feedback := <-feedbackCh:
				queue.returnPack(pack, files) // didn't use the pack during this iteration
				processFeedback(feedback.pack, feedback.files)
			}
		} else {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case feedback := <-feedbackCh:
				processFeedback(feedback.pack, feedback.files)
			}
		}
	}

	// assert restore worked correctly
	r.verifyRestoredFiles()

	return nil
}

func (r *FileRestorer) verifyRestoredFiles() {
	for _, file := range r.files {
		stat, err := os.Stat(file.path)
		if err != nil {
			fmt.Printf("Failed to restore %s", file.path)
		}
		if int64(file.node.Size) != stat.Size() {
			fmt.Printf("Wrong file size, expected %d but got %d: %s\n", file.node.Size, stat.Size(), file.path)
			offset := int64(0)
			for blobIdx, blobID := range file.node.Content {
				blobs, _ := r.repo.Index().Lookup(blobID, DataBlob)
				if len(blobs) > 1 {
					for i := 1; i < len(blobs); i++ {
						if !blobs[i].ID.Equal(blobs[0].ID) || blobs[i].Length != blobs[0].Length {
							fmt.Printf("inconsistent/unexpected blob[0] {%s, %d} != blob[%d]{%s, %d}\n", blobs[0].ID.Str(), blobs[0].Length, i, blobs[i].ID.Str(), blobs[i].Length)
						}
					}
				}
				length := blobs[0].Length - uint(crypto.Extension)
				fmt.Printf("blob #%d id %s packs %d length %d range %d-%d\n", blobIdx, blobID.Str(), len(blobs), length, offset, offset+int64(length))
				offset += int64(length)
				// TODO read file bytes and assert hash matches the blob
			}
			fmt.Printf("blobs length total %d\n", offset)
		}
	}
}

func (r *FileRestorer) downloadPack(ctx context.Context, pack *packInfo) (packCacheRecord, error) {
	const MaxInt64 = 1<<63 - 1 // odd Go does not have this predefined somewhere

	// calculate pack byte range
	start, end := int64(MaxInt64), int64(0)
	for file := range pack.files {
		r.idx.forEachFilePack(file, func(packIdx int, packID ID, packBlobs []Blob) bool {
			if packID.Equal(pack.id) {
				for _, blob := range packBlobs {
					if start > int64(blob.Offset) {
						start = int64(blob.Offset)
					}
					if end < int64(blob.Offset+blob.Length) {
						end = int64(blob.Offset + blob.Length)
					}
				}
			}

			return true // keep going
		})
	}

	cachedPack, err := r.cache.allocate(pack.id, start, (end - start))
	if err != nil {
		return cachedPack, err
	}

	if !cachedPack.hasData() {
		h := Handle{Type: DataFile, Name: pack.id.String()}
		err = r.repo.Backend().Load(ctx, h, int(cachedPack.length), cachedPack.offset, func(rd io.Reader) error {
			return cachedPack.setData(rd)
		})
		if err != nil {
			return cachedPack, err
		}
		debug.Log("Downloaded and cached pack %s (%d bytes)", cachedPack.id.Str(), cachedPack.length)
	} else {
		debug.Log("Using cached pack %s (%d bytes)", cachedPack.id.Str(), cachedPack.length)
	}

	return cachedPack, nil
}

func (r *FileRestorer) processPack(ctx context.Context, request processingInfo, cachedPack packCacheRecord) {
	defer r.cache.release(cachedPack)

	for file := range request.files {
		r.idx.forEachFilePack(file, func(packIdx int, packID ID, packBlobs []Blob) bool {
			for _, blob := range packBlobs {
				err := r.copyBlob(file, cachedPack, blob.ID, int(blob.Length), int64(blob.Offset))
				if err != nil {
					request.files[file] = err
					break // could not restore the file
				}
			}
			return false
		})
	}
}

func (r *FileRestorer) copyBlob(file *fileInfo, cachedPack packCacheRecord, blobID ID, length int, offset int64) error {
	debug.Log("Writing blob %s (%d/%d, %d bytes) from pack %s to %s", blobID.Str(), file.blobIdx+1, len(file.node.Content), length, cachedPack.id.Str(), file.path)

	// TODO reconcile with Repository#loadBlob implementation

	plaintextBuf := NewBlobBuffer(length)

	n, err := cachedPack.readAt(plaintextBuf, offset)
	if err != nil {
		return err
	}

	if n != length {
		return errors.Errorf("error loading blob %v: wrong length returned, want %d, got %d", blobID.Str(), length, n)
	}

	// decrypt
	key := r.repo.Key()
	nonce, ciphertext := plaintextBuf[:key.NonceSize()], plaintextBuf[key.NonceSize():]
	plaintext, err := key.Open(ciphertext[:0], nonce, ciphertext, nil)
	if err != nil {
		return errors.Errorf("decrypting blob %v failed: %v", blobID, err)
	}

	// check hash
	if !Hash(plaintext).Equal(blobID) {
		return errors.Errorf("blob %v returned invalid hash", blobID)
	}

	wr, err := r.acquireFileWriter(file)
	if err != nil {
		return err
	}
	n, err = wr.Write(plaintext)
	if err != nil {
		return err
	}
	if n != len(plaintext) {
		return errors.Errorf("error writing blob %v: wrong length written, want %d, got %d", blobID.Str(), length, n)
	}
	if err != nil {
		return err
	}

	return nil
}

func (r *FileRestorer) acquireFileWriter(file *fileInfo) (io.Writer, error) {
	if wr, ok := r.writers.Get(file); ok {
		return wr.(io.Writer), nil
	}
	var flags int
	if file.blobIdx > 0 {
		flags = os.O_APPEND | os.O_WRONLY
	} else {
		flags = os.O_CREATE | os.O_TRUNC | os.O_WRONLY
	}
	wr, err := os.OpenFile(file.path, flags, 0600)
	if err != nil {
		return nil, err
	}
	r.writers.Add(file, wr)
	return wr, nil
}

func (r *FileRestorer) AddFile(node *Node, dir string, path string) {
	os.MkdirAll(dir, 0755)
	file := &fileInfo{node: node, path: path}
	r.files = append(r.files, file)

	debug.Log("Added file %s", path)
	r.idx.forEachFilePack(file, func(packIdx int, packID ID, blobs []Blob) bool {
		debug.Log("   pack #%d %s", packIdx+1, packID.Str())
		for _, blob := range blobs {
			debug.Log("       blob %s %d+%d", blob.ID.Str(), blob.Offset, blob.Length)
		}
		return true
	})
}
