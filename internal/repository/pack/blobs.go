package pack

import (
	"cmp"
	"slices"
)

// Blobs is a list of blobs with pack layout information (offset, length, ...).
type Blobs []Blob

func (b Blobs) Sort() {
	slices.SortFunc(b, func(a, b Blob) int {
		return cmp.Compare(a.Offset, b.Offset)
	})
}
