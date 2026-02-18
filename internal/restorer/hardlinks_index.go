package restorer

import (
	"sync"
)

// HardlinkKey is a composite key that identifies a unique inode on a device.
type HardlinkKey struct {
	Inode, Device uint64
}

// HardlinkIndex is a mapping of unique inodes (hardlink targets) to arbitrary
// data, e.g. known names of those inodes.
type HardlinkIndex[T any] struct {
	m     sync.Mutex
	Index map[HardlinkKey]T
}

// NewHardlinkIndex create a new HardlinkIndex for a given value type.
func NewHardlinkIndex[T any]() *HardlinkIndex[T] {
	return &HardlinkIndex[T]{
		Index: make(map[HardlinkKey]T),
	}
}

// Has checks whether a given inode already exists in the index.
func (idx *HardlinkIndex[T]) Has(inode uint64, device uint64) bool {
	idx.m.Lock()
	defer idx.m.Unlock()
	_, ok := idx.Index[HardlinkKey{inode, device}]

	return ok
}

// Add adds a new inode with its accompanying data to the index, if one did not
// exist before.
func (idx *HardlinkIndex[T]) Add(inode uint64, device uint64, value T) {
	idx.m.Lock()
	defer idx.m.Unlock()
	_, ok := idx.Index[HardlinkKey{inode, device}]

	if !ok {
		idx.Index[HardlinkKey{inode, device}] = value
	}
}

// Value looks up the associated data for a given inode, and returns that data
// plus a flag indicating whether the inode exists in the index.
func (idx *HardlinkIndex[T]) Value(inode uint64, device uint64) (T, bool) {
	idx.m.Lock()
	defer idx.m.Unlock()
	v, ok := idx.Index[HardlinkKey{inode, device}]
	return v, ok
}

// Remove removes an inode from the index.
func (idx *HardlinkIndex[T]) Remove(inode uint64, device uint64) {
	idx.m.Lock()
	defer idx.m.Unlock()
	delete(idx.Index, HardlinkKey{inode, device})
}
