package restorer

import (
	"sync"
)

// HardlinkKey is a composed key for finding inodes on a specific device.
type HardlinkKey struct {
	Inode, Device uint64
}

// HardlinkIndex contains a list of inodes, devices these inodes are one, and associated file names.
type HardlinkIndex struct {
	m     sync.Mutex
	Index map[HardlinkKey]string
}

// NewHardlinkIndex create a new index for hard links
func NewHardlinkIndex() *HardlinkIndex {
	return &HardlinkIndex{
		Index: make(map[HardlinkKey]string),
	}
}

// Has checks wether the link already exist in the index.
func (idx *HardlinkIndex) Has(inode uint64, device uint64) bool {
	idx.m.Lock()
	defer idx.m.Unlock()
	_, ok := idx.Index[HardlinkKey{inode, device}]

	return ok
}

// Add adds a link to the index.
func (idx *HardlinkIndex) Add(inode uint64, device uint64, name string) {
	idx.m.Lock()
	defer idx.m.Unlock()
	_, ok := idx.Index[HardlinkKey{inode, device}]

	if !ok {
		idx.Index[HardlinkKey{inode, device}] = name
	}
}

// GetFilename obtains the filename from the index.
func (idx *HardlinkIndex) GetFilename(inode uint64, device uint64) string {
	idx.m.Lock()
	defer idx.m.Unlock()
	return idx.Index[HardlinkKey{inode, device}]
}

// Remove removes a link from the index.
func (idx *HardlinkIndex) Remove(inode uint64, device uint64) {
	idx.m.Lock()
	defer idx.m.Unlock()
	delete(idx.Index, HardlinkKey{inode, device})
}
