package restic

import (
	"container/heap"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"sync"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
)

// TODO prioritize packs with "head" blobs
// TODO do to schedule processingInfo without any target files
//      wait for feedback instead
// TODO if a blob is corrupt, there may be good blob copies in other packs
// TODO evaluate if it makes sense to cache open output files
// TODO hardlink support
// TODO evaluate memory footprint for larger repositories, say 10M packs/10M files
// TODO unexport everything
// TODO avoid decrypting the same blob multiple times

// TODO review cache is actually working, add unit tests
// TODO purge fully used up packs from the pack cache
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
	id     ID
	length int64

	data      *os.File
	populated bool
}

func newPackCache() *packCache {
	return &packCache{
		capacity:      500 * 1024 * 1024,
		reservedPacks: make(map[ID]packCacheRecord),
		cachedPacks:   make(map[ID]packCacheRecord),
	}
}

// allocates cache record for the specified pack id with the given size
// returns existing record if it is already available in the cache
func (c *packCache) allocate(packID ID, length int64) (packCacheRecord, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.reservedCapacity+length > c.capacity {
		return packCacheRecord{}, errors.Errorf("not enough cache capacity: requested %d, available %d", length, c.capacity-c.reservedCapacity)
	}

	if _, ok := c.reservedPacks[packID]; ok {
		return packCacheRecord{}, errors.Errorf("pack is already reserved %s", packID.String())
	}

	// the pack is available in the cache but currently unused
	if pack, ok := c.cachedPacks[packID]; ok {
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

	return nil
}

// ReadAt reads len(b) bytes from the File starting at byte offset off.
// It returns the number of bytes read and the error, if any.
func (r *packCacheRecord) readAt(b []byte, off int64) (n int, err error) {
	return r.data.ReadAt(b, off)
}

///////////////////////////////////////////////////////////////////////////////
// FileRestorer
///////////////////////////////////////////////////////////////////////////////

// FileRestorer restores set of files
type FileRestorer struct {
	repo  Repository
	cache *packCache

	files []*fileInfo

	downloadCh chan processingInfo
	feedbackCh chan processingInfo
}

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

type packInfo struct {
	// the pack id
	id ID

	// range of pack bytes used by this restore
	start int64
	end   int64

	// set of files that use blobs from this pack
	files map[*fileInfo]struct{}

	// number of other packs that must be downloaded before all blobs in this pack can be used
	cost int

	// used by heap
	index int
}

type processingInfo struct {
	pack  *packInfo
	files map[*fileInfo]error
}

type packQueue []*packInfo

func (pq packQueue) Len() int { return len(pq) }

func (pq packQueue) Less(a, b int) bool {
	if len(pq) <= a || len(pq) <= b {
		debug.Log("wtf len=%d a=%d b=%d", len(pq), a, b)
	}

	if pq[a].cost < pq[b].cost {
		return true
	}
	ap := inprogress(pq[a].files)
	bp := inprogress(pq[b].files)
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

func (pq packQueue) Swap(i, j int) {
	if len(pq) <= i || len(pq) <= i {
		debug.Log("wtf len=%d a=%d b=%d", len(pq), i, j)
	}

	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *packQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*packInfo)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *packQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	item.index = -1 // for safety
	*pq = old[0 : n-1]
	return item
}

type downloadInfo struct {
	pack       *packInfo
	cachedPack packCacheRecord
}

func newFileRestorer(repo Repository) *FileRestorer {
	return &FileRestorer{
		repo:       repo,
		cache:      newPackCache(),
		downloadCh: make(chan processingInfo),
		feedbackCh: make(chan processingInfo),
	}
}

func (r *FileRestorer) getBlobPack(blobID ID) (PackedBlob, error) {
	idx := r.repo.Index()
	packs, found := idx.Lookup(blobID, DataBlob)
	if !found {
		return PackedBlob{}, errors.Errorf("Unknown blob %s", blobID.String())
	}
	// TODO which pack to use if multiple packs have the blob?
	// MUST return the same pack for the same blob during the same execution
	return packs[0], nil
}

// iterates over all remaining packs of the file
func (r *FileRestorer) forEachFilePack(file *fileInfo, fn func(packIdx int, packID ID, packBlobs []Blob) bool) error {
	if file.blobIdx >= len(file.node.Content) {
		return nil
	}
	var prevPackID ID
	var prevPackBlobs []Blob
	packIdx := 0
	for _, blobID := range file.node.Content[file.blobIdx:] {
		packedBlob, err := r.getBlobPack(blobID)
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

func (r *FileRestorer) RestoreFiles(ctx context.Context) error {

	// list all files to restore
	// - calculate pack costs
	// - build pack -> file multimap

	packs := make(map[ID]*packInfo) // all packs
	queue := make(packQueue, 0)     // download queue

	const MaxInt64 = 1<<63 - 1 // odd Go does not have this predefined somewhere

	// create packInfo from fileInfo
	for _, file := range r.files {
		err := r.forEachFilePack(file, func(packIdx int, packID ID, packBlobs []Blob) bool {
			pack, ok := packs[packID]
			if !ok {
				pack = &packInfo{
					id:    packID,
					index: len(queue),
					files: make(map[*fileInfo]struct{}),
					start: MaxInt64,
				}
				queue = append(queue, pack)
				packs[packID] = pack
			}
			pack.files[file] = struct{}{}
			pack.cost += packIdx
			for _, blob := range packBlobs {
				if pack.start > int64(blob.Offset) {
					pack.start = int64(blob.Offset)
				}
				if pack.end < int64(blob.Offset+blob.Length) {
					pack.end = int64(blob.Offset + blob.Length)
				}
			}

			return true // keep going
		})
		if err != nil {
			// TODO error handling
		}
	}

	for i := 0; i < 1; i++ {
		go r.processor(ctx)
	}

	// scheduler: while files to restore
	// - find least expensive pack
	// - schedule download on another gorouting
	heap.Init(&queue)

	// NB: feedback can add more packs to the queue
	inprogress := make(fileInfoSet)
	for queue.Len()+len(inprogress) > 0 {
		debug.Log("-----------------------------------")
		debug.Log("Queue length %d, inprogress files %d", queue.Len(), len(inprogress))
		if queue.Len() > 0 {
			pack := heap.Pop(&queue).(*packInfo)
			delete(packs, pack.id)
			debug.Log("Popped pack %s (%d files), queue length=%d", pack.id.Str(), len(pack.files), len(queue))
			files := make(map[*fileInfo]error)
			for file := range pack.files {
				if !inprogress.has(file) {
					r.forEachFilePack(file, func(packIdx int, packID ID, packBlobs []Blob) bool {
						debug.Log("Pack #%d %s (%d blobs) used by %s", packIdx, packID.Str(), len(packBlobs), file.path)
						if pack.id == packID {
							files[file] = nil
							inprogress.put(file)
						}
						return false // only interested in the fist pack here
					})
				} else {
					debug.Log("Skipping inprogress %s", file.path)
				}
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case r.downloadCh <- processingInfo{pack: pack, files: files}:
				debug.Log("Scheduled download pack %s (%d files)", pack.id.Str(), len(files))
			case feedback := <-r.feedbackCh:
				// put selected pack back to the queue
				packs[pack.id] = pack
				heap.Push(&queue, pack) // put the pack back into the queue
				debug.Log("Put back to the queue popped pack %s (%d files), queue length=%d", pack.id.Str(), len(pack.files), len(queue))
				// actual feedpack processing
				debug.Log("Feedback pack %s (%d files)", feedback.pack.id.Str(), len(feedback.files))
				for file := range files {
					inprogress.remove(file)
				}
				r.processFeedback(packs, &queue, feedback)
				for file := range feedback.files {
					inprogress.remove(file)
				}
			}
		} else {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case feedback := <-r.feedbackCh:
				debug.Log("Feedback Queue length %d", queue.Len())
				r.processFeedback(packs, &queue, feedback)
				for file := range feedback.files {
					inprogress.remove(file)
				}
			}
		}
	}

	// assert restore worked
	for _, file := range r.files {
		stat, err := os.Stat(file.path)
		if err != nil {
			fmt.Printf("Failed to restore %s", file.path)
		}
		if int64(file.node.Size) != stat.Size() {
			fmt.Printf("Wrong file size, expected %d but got %d: %s", file.node.Size, stat.Size(), file.path)
		}
	}

	// downloader: while packs to download
	// - reserve required cache space
	// - download the pack
	// - schedule processing on another gorouting (why?)

	// processor: while packs to process
	// - write blobs from the pack to the awaiting files
	// - cache the pack

	return nil
}

func (r *FileRestorer) processFeedback(packs map[ID]*packInfo, queue *packQueue, feedback processingInfo) {
	otherPacks := make(map[*packInfo]struct{})
	pack := feedback.pack
	if "e1869768" == pack.id.Str() {
		debug.Log("pack %s (%d files)", pack.id.Str(), len(pack.files))
	}
	packFiles := make(map[*fileInfo]struct{}) // incomplete files still waiting for this pack
	pack.cost = 0
	for file := range pack.files {
		debug.Log("   %d/%d %s", file.blobIdx+1, len(file.node.Content), file.path)
		ferr, _ := feedback.files[file]
		if ferr != nil {
			fmt.Printf("Could not restore %s: %v\n", file.path, ferr)
			r.forEachFilePack(file, func(packIdx int, packID ID, _ []Blob) bool {
				otherPack, ok := packs[packID]
				if ok {
					otherPack.cost -= packIdx
				}
				return true
			})
			file.blobIdx = len(file.node.Content) // done with this file
			continue
		}
		err := r.forEachFilePack(file, func(packIdx int, packID ID, _ []Blob) bool {
			otherPack, ok := packs[packID]
			if ok {
				otherPack.cost--
				otherPacks[otherPack] = struct{}{}
			} else if packID.Equal(pack.id) {
				packFiles[file] = struct{}{}
				pack.cost += packIdx
			} else {
				debug.Log("Inprogress other pack %s", packID.Str())
			}
			return true // keep going
		})
		if err != nil {
			// TODO this should not be possible and pack costs are messed up if this happens
			feedback.files[file] = err
			fmt.Printf("Could not restore %s: %v\n", file.path, ferr)
			file.blobIdx = len(file.node.Content) // done with this file
		}
		if file.blobIdx >= len(file.node.Content) {
			fmt.Printf("Restored %s\n", file.path)
		}
	}
	for otherPack := range otherPacks {
		debug.Log("Adjusting cost pack %s (%d files), queue length=%d, pack index=%d", otherPack.id.Str(), len(otherPack.files), len(*queue), otherPack.index)
		heap.Fix(queue, otherPack.index)
		debug.Log("Adjusted pack cost %s (%d files), queue length=%d, new pack index=%d", otherPack.id.Str(), len(otherPack.files), len(*queue), otherPack.index)
	}
	if len(packFiles) > 0 {
		debug.Log("Queue length=%d", len(*queue))
		pack.files = packFiles
		heap.Push(queue, pack)
		packs[pack.id] = pack
		debug.Log("Requeued pack %s (%d files), queue length=%d, pack index=%d", pack.id.Str(), len(pack.files), len(*queue), pack.index)
	} else {
		debug.Log("Dropped used up pack %s", pack.id.Str())
	}
}

func (r *FileRestorer) processor(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case request := <-r.downloadCh:
			cachedPack, err := r.downloadPack(ctx, request.pack)
			if err == nil {
				r.processPack(ctx, request, cachedPack)
			} else {
				// mark all files as failed
				for file := range request.files {
					request.files[file] = err
				}
			}
			r.feedbackCh <- request
		}
	}
}

func (r *FileRestorer) downloadPack(ctx context.Context, pack *packInfo) (packCacheRecord, error) {
	cachedPack, err := r.cache.allocate(pack.id, pack.end-pack.start)
	if err != nil {
		return cachedPack, err
	}

	if !cachedPack.hasData() {
		h := Handle{Type: DataFile, Name: pack.id.String()}
		err = r.repo.Backend().Load(ctx, h, int(pack.end-pack.start), pack.start, func(rd io.Reader) error {
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
		err := r.forEachFilePack(file, func(packIdx int, packID ID, packBlobs []Blob) bool {
			for _, blob := range packBlobs {
				err := r.copyBlob(file, cachedPack, blob.ID, int(blob.Length), int64(blob.Offset)-request.pack.start)
				if err != nil {
					request.files[file] = err
					break // could not restore the file
				}
				file.blobIdx++
			}
			return false
		})
		if err != nil {
			request.files[file] = err
		}
	}
}

func (r *FileRestorer) copyBlob(file *fileInfo, cachedPack packCacheRecord, blobID ID, length int, offset int64) error {
	debug.Log("Writing blob %s (%d/%d, %d bytes) from pack %s to %s", blobID.Str(), file.blobIdx+1, len(file.node.Content), length, cachedPack.id.Str(), file.path)

	// TODO reconcile with Repository#loadBlob implementation

	plaintextBuf := NewBlobBuffer(length)

	n, err := cachedPack.readAt(plaintextBuf, offset)
	if err != nil {
		// fmt.Printf("blob %s %d+%d\n", blobID.Str(), offset, length)
		// fmt.Printf("pack=%s %d-%d tmpfile=%s\n", pack.id.Str(), pack.start, pack.end, cachedPack.data.Name())
		// for _, packedBlob := range r.repo.Index().ListPack(pack.id) {
		// 	fmt.Printf("   blob %s %d+%d\n", packedBlob.ID.Str(), packedBlob.Offset, packedBlob.Length)
		// }
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
	_, err = wr.Write(plaintext)
	if err != nil {
		r.releaseFileWriter(file, wr) // ignore secondary errors releasing the writer
		return err
	}
	err = r.releaseFileWriter(file, wr)
	if err != nil {
		return err
	}

	return nil
}

func (r *FileRestorer) acquireFileWriter(file *fileInfo) (io.Writer, error) {
	var flags int
	if file.blobIdx > 0 {
		flags = os.O_APPEND | os.O_WRONLY
	} else {
		flags = os.O_CREATE | os.O_TRUNC | os.O_WRONLY
	}
	return os.OpenFile(file.path, flags, 0600)
}

func (r *FileRestorer) releaseFileWriter(file *fileInfo, wr io.Writer) error {
	return wr.(*os.File).Close()
}

func (r *FileRestorer) AddFile(node *Node, dir string, path string) {
	os.MkdirAll(dir, 0755)
	file := &fileInfo{node: node, path: path}
	r.files = append(r.files, file)

	debug.Log("Added file %s", path)
	r.forEachFilePack(file, func(packIdx int, packID ID, blobs []Blob) bool {
		debug.Log("   pack #%d %s", packIdx+1, packID.Str())
		for _, blob := range blobs {
			debug.Log("       blob %s %d+%d", blob.ID.Str(), blob.Offset, blob.Length)
		}
		return true
	})
}
