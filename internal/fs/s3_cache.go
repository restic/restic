package fs

import (
	"cmp"
	"container/list"
	"hash/maphash"
	"math"
	"slices"
	"strings"
	"sync"
)

// s3CompactEntry is the metadata of a directory entry from a listing.
// Directories have a negative size.
type s3CompactEntry struct {
	size int64
	// mtime is in seconds since the Unix epoch
	mtime int64
	// inode is a hash of the ETag
	inode uint64
}

// s3Collision marks names which share their hash with another name in the
// same directory. They are not cached.
var s3Collision = s3CompactEntry{size: math.MinInt64}

const s3DirSize = -1

type s3CacheResult int

const (
	// s3NotCached means that the directory is not cached, or the name is
	// not cached due to a hash collision
	s3NotCached s3CacheResult = iota
	// s3Missing means that the directory listing does not contain the name
	s3Missing
	s3Found
)

// s3DirCache keeps the listings of whole directories in a compact form, the
// names themselves are only stored as hashes. Directories are evicted as a
// unit, least recently used first. The archiver walks the tree depth-first,
// so the directories which it is still working on are the most recently
// listed one and its parents. These are never evicted, however large they
// are.
type s3DirCache struct {
	mu    sync.Mutex
	hash  func(string) uint64
	limit int
	// entries is the number of entries in all cached listings
	entries int
	// lru contains *s3DirListing, most recently used first
	lru  *list.List
	dirs map[string]*list.Element
	// listed contains all directories which were listed, including those
	// which were evicted
	listed map[string]struct{}
}

// s3DirListing contains the entries of a directory sorted by the hash of
// their name. A sorted slice needs less than half the memory of a map.
type s3DirListing struct {
	path    string
	entries []s3HashedEntry
}

type s3HashedEntry struct {
	hash uint64
	s3CompactEntry
}

func (l *s3DirListing) find(h uint64) (int, bool) {
	return slices.BinarySearchFunc(l.entries, h, func(e s3HashedEntry, h uint64) int {
		return cmp.Compare(e.hash, h)
	})
}

func newS3DirCache(limit int) *s3DirCache {
	seed := maphash.MakeSeed()
	return &s3DirCache{
		hash:   func(s string) uint64 { return maphash.String(seed, s) },
		limit:  limit,
		lru:    list.New(),
		dirs:   make(map[string]*list.Element),
		listed: make(map[string]struct{}),
	}
}

// add stores the listing of dir, replacing an earlier one, and evicts other
// directories except for the parents of dir if the limit is exceeded.
func (c *s3DirCache) add(dir string, names []string, entries []s3CompactEntry) {
	hashed := make([]s3HashedEntry, len(names))
	for i, name := range names {
		hashed[i] = s3HashedEntry{hash: c.hash(name), s3CompactEntry: entries[i]}
	}
	slices.SortFunc(hashed, func(a, b s3HashedEntry) int {
		return cmp.Compare(a.hash, b.hash)
	})
	// replace entries with the same hash by a single collision marker
	n := 0
	for i := 0; i < len(hashed); i++ {
		if n > 0 && hashed[n-1].hash == hashed[i].hash {
			hashed[n-1].s3CompactEntry = s3Collision
			continue
		}
		hashed[n] = hashed[i]
		n++
	}
	listing := &s3DirListing{path: dir, entries: hashed[:n]}

	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.dirs[dir]; ok {
		c.remove(elem)
	}
	elem := c.lru.PushFront(listing)
	c.dirs[dir] = elem
	c.entries += len(listing.entries)
	c.listed[dir] = struct{}{}

	for e := c.lru.Back(); e != nil && c.entries > c.limit; {
		prev := e.Prev()
		if p := e.Value.(*s3DirListing).path; p != dir && !strings.HasPrefix(dir, p+"/") {
			c.remove(e)
		}
		e = prev
	}
}

func (c *s3DirCache) remove(elem *list.Element) {
	l := c.lru.Remove(elem).(*s3DirListing)
	delete(c.dirs, l.path)
	c.entries -= len(l.entries)
}

// get looks up name in the listing of dir. If the result is s3NotCached,
// wasListed reports whether dir was listed before and evicted since.
func (c *s3DirCache) get(dir, name string) (e s3CompactEntry, res s3CacheResult, wasListed bool) {
	h := c.hash(name)

	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.dirs[dir]
	if !ok {
		_, wasListed = c.listed[dir]
		return s3CompactEntry{}, s3NotCached, wasListed
	}
	c.lru.MoveToFront(elem)

	l := elem.Value.(*s3DirListing)
	i, ok := l.find(h)
	switch {
	case !ok:
		return s3CompactEntry{}, s3Missing, false
	case l.entries[i].s3CompactEntry == s3Collision:
		return s3CompactEntry{}, s3NotCached, false
	}
	return l.entries[i].s3CompactEntry, s3Found, false
}

// update replaces the metadata of name in the listing of dir, if cached.
func (c *s3DirCache) update(dir, name string, e s3CompactEntry) {
	h := c.hash(name)

	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.dirs[dir]
	if !ok {
		return
	}
	l := elem.Value.(*s3DirListing)
	if i, ok := l.find(h); ok && l.entries[i].s3CompactEntry != s3Collision {
		l.entries[i].s3CompactEntry = e
	}
}
