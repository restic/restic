package index

import (
	"context"
	"sort"

	"github.com/restic/restic/internal/restic"
)

type associatedSetSub[T any] struct {
	value []T
	isSet []bool
}

// AssociatedSet is a memory efficient implementation of a BlobSet that can
// store a small data item for each BlobHandle. It relies on a special property
// of our MasterIndex implementation. A BlobHandle can be permanently identified
// using an offset that never changes as MasterIndex entries cannot be modified (only added).
//
// The AssociatedSet thus can use an array with the size of the MasterIndex to store
// its data. Access to an individual entry is possible by looking up the BlobHandle's
// offset from the MasterIndex.
//
// BlobHandles that are not part of the MasterIndex can be stored by placing them in
// an overflow set that is expected to be empty in the normal case.
type AssociatedSet[T any] struct {
	byType   [restic.NumBlobTypes]associatedSetSub[T]
	overflow map[restic.BlobHandle]T
	idx      *MasterIndex
}

func NewAssociatedSet[T any](mi *MasterIndex) *AssociatedSet[T] {
	a := AssociatedSet[T]{
		overflow: make(map[restic.BlobHandle]T),
		idx:      mi,
	}

	for typ := range a.byType {
		if typ == 0 {
			continue
		}
		// index starts counting at 1
		count := mi.stableLen(restic.BlobType(typ)) + 1
		a.byType[typ].value = make([]T, count)
		a.byType[typ].isSet = make([]bool, count)
	}

	return &a
}

func (a *AssociatedSet[T]) Get(bh restic.BlobHandle) (T, bool) {
	if val, ok := a.overflow[bh]; ok {
		return val, true
	}

	idx := a.idx.blobIndex(bh)
	bt := &a.byType[bh.Type]
	if idx >= len(bt.value) || idx == -1 {
		var zero T
		return zero, false
	}

	has := bt.isSet[idx]
	if has {
		return bt.value[idx], has
	}
	var zero T
	return zero, false
}

func (a *AssociatedSet[T]) Has(bh restic.BlobHandle) bool {
	_, ok := a.Get(bh)
	return ok
}

func (a *AssociatedSet[T]) Set(bh restic.BlobHandle, val T) {
	if _, ok := a.overflow[bh]; ok {
		a.overflow[bh] = val
		return
	}

	idx := a.idx.blobIndex(bh)
	bt := &a.byType[bh.Type]
	if idx >= len(bt.value) || idx == -1 {
		a.overflow[bh] = val
	} else {
		bt.value[idx] = val
		bt.isSet[idx] = true
	}
}

func (a *AssociatedSet[T]) Insert(bh restic.BlobHandle) {
	var zero T
	a.Set(bh, zero)
}

func (a *AssociatedSet[T]) Delete(bh restic.BlobHandle) {
	if _, ok := a.overflow[bh]; ok {
		delete(a.overflow, bh)
		return
	}

	idx := a.idx.blobIndex(bh)
	bt := &a.byType[bh.Type]
	if idx < len(bt.value) && idx != -1 {
		bt.isSet[idx] = false
	}
}

func (a *AssociatedSet[T]) Len() int {
	count := 0
	a.For(func(_ restic.BlobHandle, _ T) {
		count++
	})
	return count
}

func (a *AssociatedSet[T]) For(cb func(bh restic.BlobHandle, val T)) {
	for k, v := range a.overflow {
		cb(k, v)
	}

	_ = a.idx.Each(context.Background(), func(pb restic.PackedBlob) {
		if _, ok := a.overflow[pb.BlobHandle]; ok {
			// already reported via overflow set
			return
		}

		val, known := a.Get(pb.BlobHandle)
		if known {
			cb(pb.BlobHandle, val)
		}
	})
}

// List returns a sorted slice of all BlobHandle in the set.
func (a *AssociatedSet[T]) List() restic.BlobHandles {
	list := make(restic.BlobHandles, 0)
	a.For(func(bh restic.BlobHandle, _ T) {
		list = append(list, bh)
	})

	return list
}

func (a *AssociatedSet[T]) String() string {
	list := a.List()
	sort.Sort(list)

	str := list.String()
	if len(str) < 2 {
		return "{}"
	}

	return "{" + str[1:len(str)-1] + "}"
}
