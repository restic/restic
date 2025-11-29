package rechunker

import (
	"context"
	"fmt"
	"sync"

	"github.com/hashicorp/golang-lru/v2/simplelru"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/sync/errgroup"
)

type BlobCache struct {
	mu sync.RWMutex
	c  *simplelru.LRU[restic.ID, []byte]

	free, size int

	q          map[restic.ID]packDownloadQueue // pack queue; added by requestDownload(), deleted by downloaders
	inProgress map[restic.ID]chan struct{}     // blob ready event; written by downloaders, read by asyncGet()
	downloadCh chan restic.ID                  // download request of pack ID; produced by requestDownload(), consumed by downloaders
	evictCh    chan restic.IDs                 // evict request of blob ID; produced by Ignore(), consumed by ignorer (worker)
	ignores    restic.IDSet                    // set of blobs to ignore

	blobToPack  map[restic.ID]restic.ID     // readonly: map from blob ID to pack ID the blob resides in
	packToBlobs map[restic.ID][]restic.Blob // readonly: map from pack ID to blob IDs that are used in current run

	done chan struct{}
}

type packDownloadQueue struct {
	waiter chan struct{}
	blobs  restic.IDSet
	all    bool
}

const overhead = len(restic.ID{}) + 64

func NewBlobCache(ctx context.Context, size int, numDownloaders int,
	blobToPack map[restic.ID]restic.ID, packToBlobs map[restic.ID][]restic.Blob, repo SrcRepo,
	onReady func(blobIDs restic.IDs), onEvict func(blobIDs restic.IDs)) *BlobCache {
	if size < 32*(1<<20) {
		panic("Blob cache size should be at least 32 MiB!!")
	}
	c := &BlobCache{
		size:        size,
		free:        size,
		downloadCh:  make(chan restic.ID),
		evictCh:     make(chan restic.IDs),
		q:           map[restic.ID]packDownloadQueue{},
		inProgress:  map[restic.ID]chan struct{}{},
		ignores:     restic.IDSet{},
		blobToPack:  blobToPack,
		packToBlobs: packToBlobs,
		done:        make(chan struct{}),
	}
	lru, err := simplelru.NewLRU(size, func(k restic.ID, v []byte) {
		c.free += cap(v) + overhead
		onEvict(restic.IDs{k})
	})
	if err != nil {
		panic(err)
	}
	c.c = lru

	// create download function that uses repo's LoadBlobsFromPack
	download := createDownloadFn(ctx, repo)

	startBlobCacheDownloaders(ctx, c, numDownloaders, download, onReady)

	startBlobCacheIgnorer(ctx, c)

	return c
}

type blobMap = map[restic.ID][]byte
type downloadFn func(packID restic.ID, blobs []restic.Blob) (bm blobMap, err error)

func createDownloadFn(ctx context.Context, repo SrcRepo) downloadFn {
	return func(packID restic.ID, blobs []restic.Blob) (bm blobMap, err error) {
		bm = blobMap{}
		err = repo.LoadBlobsFromPack(ctx, packID, blobs,
			func(blob restic.BlobHandle, buf []byte, err error) error {
				if err != nil {
					return err
				}
				newBuf := make([]byte, len(buf))
				copy(newBuf, buf)
				bm[blob.ID] = newBuf

				return nil
			})
		if err != nil {
			return blobMap{}, err
		}
		return bm, nil
	}
}

func startBlobCacheDownloaders(ctx context.Context, c *BlobCache, numDownloaders int, download downloadFn, onReady func(blobIDs restic.IDs)) {
	wg, ctx := errgroup.WithContext(ctx)
	for range numDownloaders {
		wg.Go(func() error {
			for {
				// listen to pack download request
				var packID restic.ID
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-c.done: // cache closed
					return nil
				case packID = <-c.downloadCh:
				}

				// retrieve pack info from the queue, and filter unneeded blobs
				// then register notification channel in c.inProgress for blobs downloaded
				c.mu.Lock()
				q := c.q[packID]
				delete(c.q, packID)
				var filtered []restic.Blob
				for _, blob := range c.packToBlobs[packID] {
					wanted := q.all || q.blobs.Has(blob.ID)
					ignored := c.ignores.Has(blob.ID)
					_, inProgress := c.inProgress[blob.ID]
					ready := c.c.Contains(blob.ID)
					if wanted && !ignored && !inProgress && !ready {
						filtered = append(filtered, blob)
					}
				}
				for _, blob := range filtered {
					c.inProgress[blob.ID] = make(chan struct{})
				}
				close(q.waiter)
				c.mu.Unlock()

				// skip if no blobs left
				if len(filtered) == 0 {
					continue
				}

				// download blobs from the repo
				bm, err := download(packID, filtered)
				if err != nil {
					return err
				}

				// save downloaded blobs to the cache
				var blobIDs restic.IDs
				c.mu.Lock()
				for id, data := range bm {
					size := cap(data) + overhead
					for size > c.free {
						c.c.RemoveOldest()
					}
					c.c.Add(id, data)
					c.free -= size
					close(c.inProgress[id])
					delete(c.inProgress, id)
					blobIDs = append(blobIDs, id)
				}
				c.mu.Unlock()

				// execute onReady callback
				if onReady != nil {
					onReady(blobIDs)
				}

				debug.Log("PackID %v loaded. Current cache usage: %v", packID.Str(), c.size-c.free)
				debug.Log("Pack %v includes the following blobs: \n%v", packID.Str(), blobIDs.String())

				// debugNote: track maximum memory usage
				debugNote.UpdateMax("max_cache_usage", c.size-c.free)
			}
		})
	}
}

func startBlobCacheIgnorer(ctx context.Context, c *BlobCache) {
	wg, ctx := errgroup.WithContext(ctx)
	wg.Go(func() error {
		for {
			// listen to ignore request (which is provided by c.Ignore())
			var ids restic.IDs
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-c.done: // cache closed
				return nil
			case ids = <-c.evictCh:
			}

			// add the blobs to ignore list and evict from cache immediately
			c.mu.Lock()
			for _, id := range ids {
				c.ignores.Insert(id)
				c.c.Remove(id)
			}
			c.mu.Unlock()

			debug.Log("Blobs %v are ignored, no longer will be downloaded", ids.String())

			// debugNote: track the number of ignored blobs
			debugNote.Add("ignored_blob_count", len(ids))
		}
	})
}

func (c *BlobCache) Get(ctx context.Context, id restic.ID, buf []byte, prefetch restic.IDs) ([]byte, <-chan []byte) {
	c.mu.Lock()
	blob, ok := c.c.Get(id) // try to retrieve blob, with recency update
	c.mu.Unlock()
	if ok { // case 1: when blob exists in cache: return that blob immediately
		if cap(buf) < len(blob) {
			debug.Log("buffer has smaller capacity than chunk size. Something might be wrong!")
			buf = make([]byte, len(blob))
		} else {
			buf = buf[:len(blob)]
		}
		copy(buf, blob)
		return buf, nil
	}

	// case 2: when blob does not exist in cache: return chOut (where downloaded blob will be delievered)
	chOut := c.asyncGet(ctx, id, buf, prefetch)

	return nil, chOut
}

func (c *BlobCache) asyncGet(ctx context.Context, id restic.ID, buf []byte, prefetch restic.IDs) <-chan []byte {
	wg, ctx := errgroup.WithContext(ctx)
	out := make(chan []byte, 1)

	wg.Go(func() error {
		for {
			c.mu.RLock()
			blob, ready := c.c.Peek(id)
			finish, inProgress := c.inProgress[id]
			c.mu.RUnlock()

			if ready { // case A: blob is now ready in the cache
				if cap(buf) < len(blob) {
					debug.Log("buffer has smaller capacity than chunk size. Something might be wrong!")
					buf = make([]byte, len(blob))
				} else {
					buf = buf[:len(blob)]
				}
				copy(buf, blob)
				out <- buf
				return nil
			}
			if inProgress { // case B: blob is being downloaded now
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-finish: // wait until download complete
					continue
				}
			}

			// case C: blob is neither in the cache nor being downloaded
			// add to the download queue
			err := c.requestDownload(ctx, id, prefetch)
			if err != nil {
				return err
			}
		}
	})

	return out
}

func (c *BlobCache) requestDownload(ctx context.Context, id restic.ID, prefetch restic.IDs) error {
	// construct valid blobs set for prefetch
	packID := c.blobToPack[id]
	valid := restic.NewIDSet(id)
	var all bool
	if prefetch == nil {
		all = true
	} else {
		for _, b := range prefetch {
			if pid := c.blobToPack[b]; packID == pid {
				valid.Insert(b)
			}
		}
	}

	c.mu.Lock()
	q, ok := c.q[packID]
	if ok { // case i: pack is in the queue: just add validBlobs to wanted list
		if all {
			q.all = true
		} else {
			q.blobs.Merge(valid)
		}
	} else { // case ii: pack is not in the queue: create new one
		q = packDownloadQueue{
			waiter: make(chan struct{}),
			blobs:  valid,
			all:    all,
		}
		c.q[packID] = q
	}
	c.mu.Unlock()

	if !ok { // if you are one who created the queue item, send packID to inform the downloader
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.done:
			return fmt.Errorf("cache closed")
		case c.downloadCh <- packID:
		}
	}

	// wait until the pack starts downloading
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return fmt.Errorf("cache closed")
	case <-q.waiter:
		return nil
	}
}

func (c *BlobCache) Ignore(ctx context.Context, blobs restic.IDs) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return fmt.Errorf("cache closed")
	case c.evictCh <- blobs:
		return nil
	}
}

func (c *BlobCache) Close() {
	if c == nil {
		return
	}

	select {
	case <-c.done:
	default:
		close(c.done)
	}
}

func createGetBlobFn(ctx context.Context, c *BlobCache) getBlobFn {
	return func(blobID restic.ID, buf []byte, prefetch restic.IDs) ([]byte, error) {
		blob, ch := c.Get(ctx, blobID, buf, prefetch)
		if blob == nil { // wait for blob to be downloaded
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case blob = <-ch:
			}
		}
		return blob, nil
	}
}

// PriorityFilesHandler is a wrapper for priority files (which are readily available in the blob cache).
type PriorityFilesHandler struct {
	filesList []*ChunkedFile
	mu        sync.Mutex
	arrival   chan struct{} // should be closed iff filesList != nil

	done chan struct{}
}

func NewPriorityFilesHandler() *PriorityFilesHandler {
	return &PriorityFilesHandler{
		arrival: make(chan struct{}),
		done:    make(chan struct{}),
	}
}

func (h *PriorityFilesHandler) Push(files []*ChunkedFile) bool {
	select {
	case <-h.done:
		return false
	default:
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	wasNil := (h.filesList == nil)
	h.filesList = append(h.filesList, files...)
	if wasNil && h.filesList != nil {
		close(h.arrival)
	}

	return true
}

func (h *PriorityFilesHandler) Arrival() <-chan struct{} {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.arrival
}

func (h *PriorityFilesHandler) Pop() []*ChunkedFile {
	select {
	case <-h.done:
		return nil
	default:
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.filesList == nil {
		return nil
	}
	l := h.filesList
	h.filesList = nil
	h.arrival = make(chan struct{})
	return l
}

func (h *PriorityFilesHandler) Done() <-chan struct{} {
	return h.done
}

func (h *PriorityFilesHandler) Close() {
	if h == nil {
		return
	}

	select {
	case <-h.done:
	default:
		close(h.done)
	}
}
