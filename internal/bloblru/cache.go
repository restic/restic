package bloblru

import (
	"sync"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"

	"github.com/hashicorp/golang-lru/simplelru"
)

// Crude estimate of the overhead per blob: a SHA-256, a linked list node
// and some pointers. See comment in Cache.add.
const overhead = len(restic.ID{}) + 64

// A Cache is a fixed-size LRU cache of blob contents.
// It is safe for concurrent access.
type Cache struct {
	mu sync.Mutex
	c  *simplelru.LRU

	free, size int // Current and max capacity, in bytes.
}

// New constructs a blob cache that stores at most size bytes worth of blobs.
func New(size int) *Cache {
	c := &Cache{
		free: size,
		size: size,
	}

	// NewLRU wants us to specify some max. number of entries, else it errors.
	// The actual maximum will be smaller than size/overhead, because we
	// evict entries (RemoveOldest in add) to maintain our size bound.
	maxEntries := size / overhead
	lru, err := simplelru.NewLRU(maxEntries, c.evict)
	if err != nil {
		panic(err) // Can only be maxEntries <= 0.
	}
	c.c = lru

	return c
}

// Add adds key id with value blob to c.
// It may return an evicted buffer for reuse.
func (c *Cache) Add(id restic.ID, blob []byte) (old []byte) {
	debug.Log("bloblru.Cache: add %v", id)

	size := cap(blob) + overhead
	if size > c.size {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	var key interface{} = id

	if c.c.Contains(key) { // Doesn't update the recency list.
		return
	}

	// This loop takes at most min(maxEntries, maxchunksize/overhead)
	// iterations.
	for size > c.free {
		_, val, _ := c.c.RemoveOldest()
		b := val.([]byte)
		if cap(b) > cap(old) {
			// We can only return one buffer, so pick the largest.
			old = b
		}
	}

	c.c.Add(key, blob)
	c.free -= size

	return old
}

func (c *Cache) Get(id restic.ID) ([]byte, bool) {
	c.mu.Lock()
	value, ok := c.c.Get(id)
	c.mu.Unlock()

	debug.Log("bloblru.Cache: get %v, hit %v", id, ok)

	blob, ok := value.([]byte)
	return blob, ok
}

func (c *Cache) evict(key, value interface{}) {
	blob := value.([]byte)
	debug.Log("bloblru.Cache: evict %v, %d bytes", key, cap(blob))
	c.free += cap(blob) + overhead
}
