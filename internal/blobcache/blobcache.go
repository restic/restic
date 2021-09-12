package blobcache

import (
	"sync"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"

	"github.com/hashicorp/golang-lru/simplelru"
)

// Crude estimate of the overhead per blob: a SHA-256, a linked list node
// and some pointers. See comment in blobCache.add.
const CacheOverhead = len(restic.ID{}) + 64

// A blobCache is a fixed-size cache of blob contents.
// It is safe for concurrent access.
type BlobCache struct {
	mu sync.Mutex
	c  *simplelru.LRU

	free, size int // Current and max capacity, in bytes.
}

// Construct a blob cache that stores at most size bytes worth of blobs.
func New(size int) *BlobCache {
	c := &BlobCache{
		free: size,
		size: size,
	}

	// NewLRU wants us to specify some max. number of entries, else it errors.
	// The actual maximum will be smaller than size/CacheOverhead, because we
	// evict entries (RemoveOldest in add) to maintain our size bound.
	maxEntries := size / CacheOverhead
	lru, err := simplelru.NewLRU(maxEntries, c.evict)
	if err != nil {
		panic(err) // Can only be maxEntries <= 0.
	}
	c.c = lru

	return c
}

func (c *BlobCache) Add(id restic.ID, blob []byte) {
	debug.Log("blobCache: add %v", id)

	size := len(blob) + CacheOverhead
	if size > c.size {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	var key interface{} = id

	if c.c.Contains(key) { // Doesn't update the recency list.
		return
	}

	// This loop takes at most min(maxEntries, maxchunksize/CacheOverhead)
	// iterations.
	for size > c.free {
		c.c.RemoveOldest()
	}

	c.c.Add(key, blob)
	c.free -= size
}

func (c *BlobCache) Get(id restic.ID) ([]byte, bool) {
	c.mu.Lock()
	value, ok := c.c.Get(id)
	c.mu.Unlock()

	debug.Log("BlobCache: get %v, hit %v", id, ok)

	blob, ok := value.([]byte)
	return blob, ok
}

func (c *BlobCache) evict(key, value interface{}) {
	blob := value.([]byte)
	debug.Log("BlobCache: evict %v, %d bytes", key, len(blob))
	c.free += len(blob) + CacheOverhead
}
