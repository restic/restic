package restorer

import (
	"sync"
)

// hardlinkKey is a composed key for finding inodes on a specific device.
type hardlinkKey struct {
	Inode, Device uint64
}

// HardlinkIndex maps inodes on devices to associated values.
type HardlinkIndex[T any] struct {
	m     sync.Mutex
	index map[hardlinkKey]T
}

// NewHardlinkIndex create a new index for hard links
func NewHardlinkIndex[T any]() *HardlinkIndex[T] {
	return &HardlinkIndex[T]{
		index: make(map[hardlinkKey]T),
	}
}

// Has checks whether the link already exist in the index.
func (idx *HardlinkIndex[T]) Has(inode uint64, device uint64) bool {
	idx.m.Lock()
	defer idx.m.Unlock()
	_, ok := idx.index[hardlinkKey{inode, device}]

	return ok
}

// Add adds a link to the index.
func (idx *HardlinkIndex[T]) Add(inode uint64, device uint64, value T) {
	idx.m.Lock()
	defer idx.m.Unlock()
	_, ok := idx.index[hardlinkKey{inode, device}]

	if !ok {
		idx.index[hardlinkKey{inode, device}] = value
	}
}

// Value obtains the filename from the index.
func (idx *HardlinkIndex[T]) Value(inode uint64, device uint64) T {
	idx.m.Lock()
	defer idx.m.Unlock()
	return idx.index[hardlinkKey{inode, device}]
}

// Remove removes a link from the index.
func (idx *HardlinkIndex[T]) Remove(inode uint64, device uint64) {
	idx.m.Lock()
	defer idx.m.Unlock()
	delete(idx.index, hardlinkKey{inode, device})
}
