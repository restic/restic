package restorer

import (
	"github.com/puzpuzpuz/xsync/v3"
	"github.com/restic/restic/internal/restic"
)

// ReflinkKey is a composite key that identifies a unique file content.
type ReflinkKey struct {
	Content restic.IDs
}

// ReflinkItem is a structure that describes a specific file in the snapshot.
type ReflinkItem struct {
	Name string
}

// ReflinkValue is a structure describing all known files with the same content
// that are being extracted.
type ReflinkValue struct {
	entry ReflinkItem
	//m       xsync.RBMutex
	//entries []ReflinkItem
}

// ReflinkIndex is an index of all unique file contents to the files that have
// this content, for the purposes of reflink tracking.
// For now, this structure only tracks the first file with given content, as we
// will only attempt to extract one file and clone all others from it.
type ReflinkIndex struct {
	Index *xsync.MapOf[ReflinkKey, ReflinkValue]
}

// idHasher is the generated hasher function for a single blob ID.
var idHasher func(restic.ID, uint64) uint64

func init() {
	idHasher = xsync.Hasher[restic.ID]()
}

// reflinkKeyHasher is the hashing function for a given ReflinkKey, implemented
// by chain-hashing all blob IDs. This is needed because Go cannot hash slices.
func reflinkKeyHasher(key ReflinkKey, seed uint64) uint64 {
	h := seed
	for _, i := range key.Content {
		h = idHasher(i, h)
	}
	return h
}

// reflinkKeyCmp is the comparator function for a given ReflinkKey.
// This is needed because Go cannot compare slices.
func reflinkKeyCmp(a ReflinkKey, b ReflinkKey) bool {
	if a.Content.Len() != b.Content.Len() {
		return false
	}
	for i, v1 := range a.Content {
		v2 := b.Content[i]
		if v1 != v2 {
			return false
		}
	}
	return true
}

// NewReflinkIndex creates a new ReflinkIndex with fixed key and value types.
func NewReflinkIndex() *ReflinkIndex {
	return &ReflinkIndex{
		Index: xsync.NewMapOfWithMethods[ReflinkKey, ReflinkValue](
			reflinkKeyHasher,
			reflinkKeyCmp,
		),
	}
}

// Put inserts a new mapping from file content to its name. A mapping is only
// created if given content is not yet known, causing ReflinkIndex to track
// names of the files considered "originals" for any given content.
// The name of  the "original" is returned, along with the flag indicating
// whether _this_ invocation has inserted an original.
func (idx *ReflinkIndex) Put(name string, ids restic.IDs) (orig string, isOrig bool) {
	v, loaded := idx.Index.LoadOrStore(
		ReflinkKey{ids},
		ReflinkValue{ReflinkItem{name}},
	)
	return v.entry.Name, !loaded
}

// Get looks up the associated file name that is "original" for a given content,
// and returns that file name plus a flag indicating whether the original exists
// in the index.
func (idx *ReflinkIndex) Get(ids restic.IDs) (orig string, hasOrig bool) {
	v, loaded := idx.Index.Load(
		ReflinkKey{ids},
	)
	return v.entry.Name, loaded
}
