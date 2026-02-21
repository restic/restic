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

	idx *Index

	free, size int

	waitList   restic.IDSet                // set of packs waiting for download
	inProgress map[restic.ID]chan struct{} // blob ready event channel; open by requestDownload(), closed by downloaders
	downloadCh chan restic.ID              // pack download request channel; produced by requestDownload(), consumed by downloaders

	ignored restic.IDSet // set of ignored blobs; blobs in this set are excluded from download

	cancel func() // this function is called at Close(), cancelling cache context
}

const overhead = len(restic.ID{}) + 64

func NewBlobCache(ctx context.Context, size int, numDownloaders int,
	repo PackLoader, idx *Index,
	onReady func(blobIDs restic.IDs), onEvict func(blobIDs restic.IDs)) *BlobCache {
	if size < 32*(1<<20) {
		panic("Blob cache size should be at least 32 MiB!!")
	}
	debug.Log("Creating blob cache of size %v", size)

	ctx, cancel := context.WithCancel(ctx)

	c := &BlobCache{
		idx: idx,

		size: size,
		free: size,

		waitList:   restic.NewIDSet(),
		inProgress: map[restic.ID]chan struct{}{},
		downloadCh: make(chan restic.ID),

		ignored: restic.NewIDSet(),

		cancel: cancel,
	}

	lru, err := simplelru.NewLRU(size, func(_ restic.ID, v []byte) {
		c.free += cap(v) + overhead
	})
	if err != nil {
		panic(err)
	}
	c.c = lru

	// create download function that uses repo's LoadBlobsFromPack
	download := createDownloadFn(ctx, repo)

	c.startDownloaders(ctx, numDownloaders, download, onReady, onEvict)

	return c
}

type blobMap = map[restic.ID][]byte
type downloadFn func(packID restic.ID, blobs []restic.Blob) (blobMap, error)

func createDownloadFn(ctx context.Context, repo PackLoader) downloadFn {
	return func(packID restic.ID, blobs []restic.Blob) (blobMap, error) {
		bm := blobMap{}
		err := repo.LoadBlobsFromPack(ctx, packID, blobs,
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

func (c *BlobCache) startDownloaders(ctx context.Context, numDownloaders int,
	download downloadFn, onReady, onEvict func(blobIDs restic.IDs)) {
	wg, ctx := errgroup.WithContext(ctx)
	for range numDownloaders {
		wg.Go(func() error {
			debug.Log("Starting blob cache downloader")
			defer debug.Log("Stopping blob cache downloader")

			for {
				// listen to pack download request
				var packID restic.ID
				select {
				case <-ctx.Done():
					return ctx.Err()
				case packID = <-c.downloadCh:
				}

				// filter out ignored blobs
				c.mu.RLock()
				var filtered []restic.Blob
				for _, blob := range c.idx.PackToBlobs[packID] {
					ignored := c.ignored.Has(blob.ID)
					ready := c.c.Contains(blob.ID)
					if !ignored && !ready {
						filtered = append(filtered, blob)
					}
				}
				c.mu.RUnlock()

				// skip if no blobs to download
				if len(filtered) == 0 {
					continue
				}

				// download blobs from the repo
				debug.Log("Starting download of %v blobs in pack %v", len(filtered), packID.Str())
				blobs, err := download(packID, filtered)
				if err != nil {
					return err
				}

				// pop the pack from the waitlist,
				// store downloaded blobs to the cache,
				// and notify that blobs are ready
				var ready, evicted restic.IDs
				c.mu.Lock()
				delete(c.waitList, packID)
				for id, data := range blobs {
					size := cap(data) + overhead
					for size > c.free { // evict old blobs if there is not enough free space
						id, _, ok := c.c.RemoveOldest()
						if ok {
							evicted = append(evicted, id)
						} else {
							defer c.mu.Unlock()
							return fmt.Errorf("not enough cache size to store a blob; needs at least %d bytes, but has only %d bytes", size, c.free)
						}
					}
					c.c.Add(id, data)
					c.free -= size
					if _, ok := c.inProgress[id]; ok {
						close(c.inProgress[id])
						delete(c.inProgress, id)
					}
					ready = append(ready, id)
				}
				currentCacheUsage := c.size - c.free // for debug logging
				c.mu.Unlock()

				// execute callbacks
				if len(evicted) > 0 {
					if onEvict != nil {
						onEvict(evicted)
					}
					debug.Log("%v blobs are evicted.", len(evicted))
				}
				if onReady != nil {
					onReady(ready)
				}

				debug.Log("Pack %v loaded. Current cache usage: %v", packID.Str(), currentCacheUsage)
				debug.Log("Pack %v includes the following blobs: \n%v", packID.Str(), ready.String())

				// debugStats: track maximum memory usage
				if debugStats != nil {
					debugStats.UpdateMax("max_cache_usage", currentCacheUsage)
				}
			}
		})
	}
}

func (c *BlobCache) Get(ctx context.Context, id restic.ID, buf []byte) ([]byte, <-chan []byte) {
	c.mu.Lock()
	blob, ok := c.c.Get(id) // try to retrieve blob, with recency update
	c.mu.Unlock()

	if ok { // case 1: when blob exists in cache: return that blob immediately
		if cap(buf) < len(blob) {
			debug.Log("Allocating new buf, as it has smaller capacity than chunk size.")
			buf = make([]byte, len(blob))
		} else {
			buf = buf[:len(blob)]
		}
		copy(buf, blob)

		debug.Log("Cache hit. Returning blob %v", id.Str())
		return buf, nil
	}

	// case 2: when blob does not exist in cache: return chOut (where downloaded blob will be delievered)
	debug.Log("Cache miss. Requesting async get for blob %v", id.Str())
	chOut := c.asyncGet(ctx, id, buf)

	return nil, chOut
}

func (c *BlobCache) asyncGet(ctx context.Context, id restic.ID, buf []byte) <-chan []byte {
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
					debug.Log("Allocating new buf, as it has smaller capacity than chunk size.")
					buf = make([]byte, len(blob))
				} else {
					buf = buf[:len(blob)]
				}
				copy(buf, blob)

				debug.Log("Blob %v is now ready in the cache. Passing blob data to channel.", id.Str())
				out <- buf
				return nil
			}
			if inProgress { // case B: blob is queued, but not yet ready
				debug.Log("Waiting for blob %v to be ready in the cache.", id.Str())
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-finish: // wait until download complete
					continue
				}
			}

			// case C: blob is not queued
			// add to the download queue
			debug.Log("Requesting download of the pack containing blob %v", id.Str())
			err := c.requestDownload(ctx, id)
			if err != nil {
				return err
			}
		}
	})

	return out
}

func (c *BlobCache) requestDownload(ctx context.Context, id restic.ID) error {
	packID, ok := c.idx.BlobToPack[id]
	if !ok {
		return fmt.Errorf("unknown blob: %v", id.Str())
	}

	c.mu.Lock()
	ok = c.waitList.Has(packID)
	if !ok {
		// queue pack download
		c.waitList.Insert(packID)
	}
	if _, inProgress := c.inProgress[id]; !inProgress {
		c.inProgress[id] = make(chan struct{})
	}
	c.mu.Unlock()

	if ok { // somebody else has already queued pack download; it will handle download request
		return nil
	}

	// send packID to inform the downloader
	select {
	case <-ctx.Done():
		return ctx.Err()
	case c.downloadCh <- packID:
		return nil
	}
}

func (c *BlobCache) Ignore(ids restic.IDs) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, id := range ids {
		c.ignored.Insert(id)
		_ = c.c.Remove(id)
	}

	if debugStats != nil {
		debugStats.Add("ignored_blob_count", len(ids))
	}
}

func (c *BlobCache) Close() {
	if c == nil {
		return
	}

	c.cancel()
}

type BlobLoaderWithCache struct {
	repo  PackLoader
	cache *BlobCache
}

func (l *BlobLoaderWithCache) LoadBlob(ctx context.Context, _ restic.BlobType, id restic.ID, buf []byte) ([]byte, error) {
	blob, ch := l.cache.Get(ctx, id, buf)
	if blob == nil { // wait for blob to be downloaded
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case blob = <-ch:
		}
	}
	return blob, nil
}

type PackLoader interface {
	LoadBlobsFromPack(context.Context, restic.ID, []restic.Blob, func(restic.BlobHandle, []byte, error) error) error
}

func WrapWithCache(ctx context.Context, repo PackLoader, cacheSize int, numDownloaders int, idx *Index,
	onReady, onEvict func(restic.IDs)) (*BlobLoaderWithCache, *BlobCache) {
	r := &BlobLoaderWithCache{
		repo:  repo,
		cache: NewBlobCache(ctx, cacheSize, numDownloaders, repo, idx, onReady, onEvict),
	}

	debug.Log("Wrapped the repository with blob cache.")
	return r, r.cache
}
