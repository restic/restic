package archiver

// Virtual device ID mapping: converts real device IDs into a stable virtual
// numbering 1..n, where assignment order is determined by first-seen. This
// keeps snapshot trees stable across runs regardless of which devices appear
// or in what order.
//
// Concurrency: only the goroutine that created the mapper via
// newMutableDeviceIdMapper() may call GetVirtualId on that mapper. For that
// goroutine, GetVirtualId may allocate a new virtual ID for an unseen device.
// All other goroutines must use the result of ReadOnlyMapper(). On the
// read-only mapper, GetVirtualId returns (0, false) for any device ID not yet
// seen by the mutable owner.
//
// See deviceIdMapper and newMutableDeviceIdMapper.

import (
	"sync"

	"github.com/restic/restic/internal/debug"
)

// deviceIdMapper maps real device IDs to stable virtual IDs. Implementations
// are either mutable (may assign new virtual IDs) or read-only (lookup only).
type deviceIdMapper interface {
	GetVirtualId(realDeviceID uint64) (uint64, bool)
	ReadOnlyMapper() deviceIdMapper
}

// newMutableDeviceIdMapper returns a mapper that may assign new virtual IDs.
// Only the creating goroutine may call GetVirtualId on the returned value.
func newMutableDeviceIdMapper() deviceIdMapper {
	m := &deviceIdMap{
		count: 0,
		cache: &vIdCache{},
	}
	m.cache.set(0, 0)
	return m
}

// vIdCache is a concurrent read-only view of realID -> virtualID; GetVirtualId returns (0, false) when missing.
type vIdCache sync.Map

func (v *vIdCache) GetVirtualId(realDeviceID uint64) (uint64, bool) {
	if id, ok := (*sync.Map)(v).Load(realDeviceID); ok {
		return id.(uint64), true
	}
	return 0, false
}

func (v *vIdCache) set(realDeviceID uint64, virtualDeviceID uint64) {
	(*sync.Map)(v).Store(realDeviceID, virtualDeviceID)
}

func (v *vIdCache) ReadOnlyMapper() deviceIdMapper {
	return v
}

// deviceIdMap is the mutable mapper; only its owner goroutine may call GetVirtualId.
type deviceIdMap struct {
	count uint64
	cache *vIdCache
}

// GetVirtualId returns the virtual ID for realDeviceID, allocating one (first-seen order) if needed.
func (d *deviceIdMap) GetVirtualId(realDeviceID uint64) (uint64, bool) {
	id, ok := d.cache.GetVirtualId(realDeviceID)
	if !ok {
		id = d.count + 1
		d.cache.set(realDeviceID, id)
		d.count = id
		debug.Log("Mapped deviceId %v to virtualId %v", realDeviceID, id)
	}
	return id, true
}

// ReadOnlyMapper returns a concurrent-safe view for other goroutines; they must use this, not GetVirtualId on d.
func (d *deviceIdMap) ReadOnlyMapper() deviceIdMapper {
	return d.cache
}
