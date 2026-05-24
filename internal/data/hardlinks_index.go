package data

import (
	"sync"
)

// hardlinkKey is a composite key that identifies a unique inode on a device.
type hardlinkKey struct {
	Inode, Device uint64
}

// HardlinkIndex maps unique inodes (hardlink targets) to arbitrary data,
// e.g. known names of those inodes.
type HardlinkIndex[T any] struct {
	m     sync.Mutex
	index map[hardlinkKey]T
}

// NewHardlinkIndex create a new HardlinkIndex for a given value type.
func NewHardlinkIndex[T any]() *HardlinkIndex[T] {
	return &HardlinkIndex[T]{
		index: make(map[hardlinkKey]T),
	}
}

// Has checks whether a given inode already exists in the index.
func (idx *HardlinkIndex[T]) Has(inode uint64, device uint64) bool {
	idx.m.Lock()
	defer idx.m.Unlock()
	_, ok := idx.index[hardlinkKey{inode, device}]

	return ok
}

// Add adds a new inode with its accompanying data to the index, if one did not
// exist before.
func (idx *HardlinkIndex[T]) Add(inode uint64, device uint64, value T) {
	idx.m.Lock()
	defer idx.m.Unlock()
	_, ok := idx.index[hardlinkKey{inode, device}]

	if !ok {
		idx.index[hardlinkKey{inode, device}] = value
	}
}

// Value looks up the associated data for a given inode, and returns that data
// plus a flag indicating whether the inode exists in the index.
func (idx *HardlinkIndex[T]) Value(inode uint64, device uint64) (T, bool) {
	idx.m.Lock()
	defer idx.m.Unlock()
	v, ok := idx.index[hardlinkKey{inode, device}]
	return v, ok
}

// Remove removes an inode from the index.
func (idx *HardlinkIndex[T]) Remove(inode uint64, device uint64) {
	idx.m.Lock()
	defer idx.m.Unlock()
	delete(idx.index, hardlinkKey{inode, device})
}
