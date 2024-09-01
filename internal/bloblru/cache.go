package bloblru

import (
	"sync"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"

	"github.com/hashicorp/golang-lru/v2/simplelru"
)

// Crude estimate of the overhead per blob: a SHA-256, a linked list node
// and some pointers. See comment in Cache.add.
const overhead = len(restic.ID{}) + 64

// A Cache is a fixed-size LRU cache of blob contents.
// It is safe for concurrent access.
type Cache struct {
	mu sync.Mutex
	c  *simplelru.LRU[restic.ID, []byte]

	free, size int // Current and max capacity, in bytes.
	inProgress map[restic.ID]chan struct{}
}

// New constructs a blob cache that stores at most size bytes worth of blobs.
func New(size int) *Cache {
	c := &Cache{
		free:       size,
		size:       size,
		inProgress: make(map[restic.ID]chan struct{}),
	}

	// NewLRU wants us to specify some max. number of entries, else it errors.
	// The actual maximum will be smaller than size/overhead, because we
	// evict entries (RemoveOldest in add) to maintain our size bound.
	maxEntries := size / overhead
	lru, err := simplelru.NewLRU[restic.ID, []byte](maxEntries, c.evict)
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

	if c.c.Contains(id) { // Doesn't update the recency list.
		return
	}

	// This loop takes at most min(maxEntries, maxchunksize/overhead)
	// iterations.
	for size > c.free {
		_, b, _ := c.c.RemoveOldest()
		if cap(b) > cap(old) {
			// We can only return one buffer, so pick the largest.
			old = b
		}
	}

	c.c.Add(id, blob)
	c.free -= size

	return old
}

func (c *Cache) Get(id restic.ID) ([]byte, bool) {
	c.mu.Lock()
	blob, ok := c.c.Get(id)
	c.mu.Unlock()

	debug.Log("bloblru.Cache: get %v, hit %v", id, ok)

	return blob, ok
}

func (c *Cache) GetOrCompute(id restic.ID, compute func() ([]byte, error)) ([]byte, error) {
	// check if already cached
	blob, ok := c.Get(id)
	if ok {
		return blob, nil
	}

	// check for parallel download or start our own
	finish := make(chan struct{})
	c.mu.Lock()
	waitForResult, isComputing := c.inProgress[id]
	if !isComputing {
		c.inProgress[id] = finish
	}
	c.mu.Unlock()

	if isComputing {
		// wait for result of parallel download
		<-waitForResult
	} else {
		// remove progress channel once finished here
		defer func() {
			c.mu.Lock()
			delete(c.inProgress, id)
			c.mu.Unlock()
			close(finish)
		}()
	}

	// try again. This is necessary independent of whether isComputing is true or not.
	// The calls to `c.Get()` and checking/adding the entry in `c.inProgress` are not atomic,
	// thus the item might have been computed in the meantime.
	// The following scenario would compute() the value multiple times otherwise:
	// Goroutine A does not find a value in the initial call to `c.Get`, then goroutine B
	// takes over, caches the computed value and cleans up its channel in c.inProgress.
	// Then goroutine A continues, does not detect a parallel computation and would try
	// to call compute() again.
	blob, ok = c.Get(id)
	if ok {
		return blob, nil
	}

	// download it
	blob, err := compute()
	if err == nil {
		c.Add(id, blob)
	}

	return blob, err
}

func (c *Cache) evict(key restic.ID, blob []byte) {
	debug.Log("bloblru.Cache: evict %v, %d bytes", key, cap(blob))
	c.free += cap(blob) + overhead
}
