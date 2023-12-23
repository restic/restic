package restorer

import (
	"sync"
)

// HardlinkKey is a composed key for finding inodes on a specific device.
type HardlinkKey struct {
	Inode, Device uint64
}

// HardlinkIndex contains a list of inodes, devices these inodes are one, and associated file names.
type HardlinkIndex[T any] struct {
	m     sync.Mutex
	Index map[HardlinkKey]T
}

// NewHardlinkIndex create a new index for hard links
func NewHardlinkIndex[T any]() *HardlinkIndex[T] {
	return &HardlinkIndex[T]{
		Index: make(map[HardlinkKey]T),
	}
}

// Has checks whether the link already exist in the index.
func (idx *HardlinkIndex[T]) Has(inode uint64, device uint64) bool {
	idx.m.Lock()
	defer idx.m.Unlock()
	_, ok := idx.Index[HardlinkKey{inode, device}]

	return ok
}

// Add adds a link to the index.
func (idx *HardlinkIndex[T]) Add(inode uint64, device uint64, value T) {
	idx.m.Lock()
	defer idx.m.Unlock()
	_, ok := idx.Index[HardlinkKey{inode, device}]

	if !ok {
		idx.Index[HardlinkKey{inode, device}] = value
	}
}

// Value obtains the filename from the index.
func (idx *HardlinkIndex[T]) Value(inode uint64, device uint64) T {
	idx.m.Lock()
	defer idx.m.Unlock()
	return idx.Index[HardlinkKey{inode, device}]
}

// Remove removes a link from the index.
func (idx *HardlinkIndex[T]) Remove(inode uint64, device uint64) {
	idx.m.Lock()
	defer idx.m.Unlock()
	delete(idx.Index, HardlinkKey{inode, device})
}
