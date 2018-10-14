package restorer

import (
	"io"
	"sync"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
)

// packCache is thread safe in-memory cache of pack files required to restore
// one or more files. The cache is meant to hold pack files that cannot be
// fully used right away. This happens when pack files contains blobs from
// "head" of some files and "middle" of other files. "Middle" blobs cannot be
// written to their files until after blobs from some other packs are written
// to the files first.
//
// While the cache is thread safe, implementation assumes (and enforces)
// that individual entries are used by one client at a time. Clients must
// #Close() entry's reader to make the entry available for use by other
// clients. This limitation can be relaxed in the future if necessary.
type packCache struct {
	// guards access to cache internal data structures
	lock sync.Mutex

	// cache capacity
	capacity          int
	reservedCapacity  int
	allocatedCapacity int

	// pack records currently being used by active restore worker
	reservedPacks map[restic.ID]*packCacheRecord

	// unused allocated packs, can be deleted if necessary
	cachedPacks map[restic.ID]*packCacheRecord
}

type packCacheRecord struct {
	master *packCacheRecord
	cache  *packCache

	id     restic.ID // cached pack id
	offset int64     // cached pack byte range

	data []byte
}

type readerAtCloser interface {
	io.Closer
	io.ReaderAt
}

type bytesWriteSeeker struct {
	pos  int
	data []byte
}

func (wr *bytesWriteSeeker) Write(p []byte) (n int, err error) {
	if wr.pos+len(p) > len(wr.data) {
		return -1, errors.Errorf("not enough space")
	}
	n = copy(wr.data[wr.pos:], p)
	wr.pos += n
	return n, nil
}

func (wr *bytesWriteSeeker) Seek(offset int64, whence int) (int64, error) {
	if offset != 0 || whence != io.SeekStart {
		return -1, errors.Errorf("unsupported seek request")
	}
	wr.pos = 0
	return 0, nil
}

func newPackCache(capacity int) *packCache {
	return &packCache{
		capacity:      capacity,
		reservedPacks: make(map[restic.ID]*packCacheRecord),
		cachedPacks:   make(map[restic.ID]*packCacheRecord),
	}
}

func (c *packCache) reserve(packID restic.ID, offset int64, length int) (record *packCacheRecord, err error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if offset < 0 || length <= 0 {
		return nil, errors.Errorf("illegal pack cache allocation range %s {offset: %d, length: %d}", packID.Str(), offset, length)
	}

	if c.reservedCapacity+length > c.capacity {
		return nil, errors.Errorf("not enough cache capacity: requested %d, available %d", length, c.capacity-c.reservedCapacity)
	}

	if _, ok := c.reservedPacks[packID]; ok {
		return nil, errors.Errorf("pack is already reserved %s", packID.Str())
	}

	// the pack is available in the cache and currently unused
	if pack, ok := c.cachedPacks[packID]; ok {
		// check if cached pack includes requested byte range
		// the range can shrink, but it never grows bigger unless there is a bug elsewhere
		if pack.offset > offset || (pack.offset+int64(len(pack.data))) < (offset+int64(length)) {
			return nil, errors.Errorf("cached range %d-%d is smaller than requested range %d-%d for pack %s", pack.offset, pack.offset+int64(len(pack.data)), length, offset+int64(length), packID.Str())
		}

		// move the pack to the used map
		delete(c.cachedPacks, packID)
		c.reservedPacks[packID] = pack
		c.reservedCapacity += len(pack.data)

		debug.Log("Using cached pack %s (%d bytes)", pack.id.Str(), len(pack.data))

		if pack.offset != offset || len(pack.data) != length {
			// restrict returned record to requested range
			return &packCacheRecord{
				cache:  c,
				master: pack,
				offset: offset,
				data:   pack.data[int(offset-pack.offset) : int(offset-pack.offset)+length],
			}, nil
		}

		return pack, nil
	}

	for c.allocatedCapacity+length > c.capacity {
		// all cached packs will be needed at some point
		// so it does not matter which one to purge
		for _, cached := range c.cachedPacks {
			delete(c.cachedPacks, cached.id)
			c.allocatedCapacity -= len(cached.data)
			debug.Log("dropped cached pack %s (%d bytes)", cached.id.Str(), len(cached.data))
			break
		}
	}

	pack := &packCacheRecord{
		cache:  c,
		id:     packID,
		offset: offset,
	}
	c.reservedPacks[pack.id] = pack
	c.allocatedCapacity += length
	c.reservedCapacity += length

	return pack, nil
}

// get returns reader of the specified cached pack. Uses provided load func
// to download pack content if necessary.
// The returned reader is only able to read pack within byte range specified
// by offset and length parameters, attempts to read outside that range will
// result in an error.
// The returned reader must be closed before the same packID can be requested
// from the cache again.
func (c *packCache) get(packID restic.ID, offset int64, length int, load func(offset int64, length int, wr io.WriteSeeker) error) (readerAtCloser, error) {
	pack, err := c.reserve(packID, offset, length)
	if err != nil {
		return nil, err
	}

	if pack.data == nil {
		releasePack := func() {
			delete(c.reservedPacks, pack.id)
			c.reservedCapacity -= length
			c.allocatedCapacity -= length
		}
		wr := &bytesWriteSeeker{data: make([]byte, length)}
		err = load(offset, length, wr)
		if err != nil {
			releasePack()
			return nil, err
		}
		if wr.pos != length {
			releasePack()
			return nil, errors.Errorf("invalid read size")
		}
		pack.data = wr.data
		debug.Log("Downloaded and cached pack %s (%d bytes)", pack.id.Str(), len(pack.data))
	}

	return pack, nil
}

// releases the pack record back to the cache
func (c *packCache) release(pack *packCacheRecord) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	if _, ok := c.reservedPacks[pack.id]; !ok {
		return errors.Errorf("invalid pack release request")
	}

	delete(c.reservedPacks, pack.id)
	c.cachedPacks[pack.id] = pack
	c.reservedCapacity -= len(pack.data)

	return nil
}

// remove removes specified pack from the cache and frees
// corresponding cache space. should be called after the pack
// was fully used up by the restorer.
func (c *packCache) remove(packID restic.ID) error {
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
	c.allocatedCapacity -= len(pack.data)

	return nil
}

// ReadAt reads len(b) bytes from the pack starting at byte offset off.
// It returns the number of bytes read and the error, if any.
func (r *packCacheRecord) ReadAt(b []byte, off int64) (n int, err error) {
	if off < r.offset || off+int64(len(b)) > r.offset+int64(len(r.data)) {
		return -1, errors.Errorf("read outside available range")
	}
	return copy(b, r.data[off-r.offset:]), nil
}

// Close closes the pack reader and releases corresponding cache record
// to the cache. Once closed, the record can be reused by subsequent
// requests for the same packID or it can be purged from the cache to make
// room for other packs
func (r *packCacheRecord) Close() (err error) {
	if r.master != nil {
		return r.cache.release(r.master)
	}
	return r.cache.release(r)
}
