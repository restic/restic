package index

import (
	"context"

	"github.com/restic/restic/internal/restic"
)

type associatedDataSub[T any] struct {
	value []T
	isSet []bool
}

type AssociatedData[T any] struct {
	byType   [restic.NumBlobTypes]associatedDataSub[T]
	overflow map[restic.BlobHandle]T
	idx      *MasterIndex
}

func NewAssociated[T any](mi *MasterIndex) *AssociatedData[T] {
	a := AssociatedData[T]{
		overflow: make(map[restic.BlobHandle]T),
		idx:      mi,
	}

	for typ := range a.byType {
		if typ == 0 {
			continue
		}
		count := mi.len(restic.BlobType(typ))
		a.byType[typ].value = make([]T, count)
		a.byType[typ].isSet = make([]bool, count)
	}

	return &a
}

func (a *AssociatedData[T]) Get(bh restic.BlobHandle) (T, bool) {
	if val, ok := a.overflow[bh]; ok {
		return val, true
	}

	idx := a.idx.getBlobIndex(bh)
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

func (a *AssociatedData[T]) Has(bh restic.BlobHandle) bool {
	_, ok := a.Get(bh)
	return ok
}

func (a *AssociatedData[T]) Set(bh restic.BlobHandle, val T) {
	a.set(bh, val)
}

func (a *AssociatedData[T]) set(bh restic.BlobHandle, val T) {
	if _, ok := a.overflow[bh]; ok {
		a.overflow[bh] = val
		return
	}

	idx := a.idx.getBlobIndex(bh)
	bt := &a.byType[bh.Type]
	if idx >= len(bt.value) || idx == -1 {
		a.overflow[bh] = val
	} else {
		bt.value[idx] = val
		bt.isSet[idx] = true
	}
}

func (a *AssociatedData[T]) Insert(bh restic.BlobHandle) {
	var zero T
	a.set(bh, zero)
}

func (a *AssociatedData[T]) Delete(bh restic.BlobHandle) {
	if _, ok := a.overflow[bh]; ok {
		delete(a.overflow, bh)
		return
	}

	idx := a.idx.getBlobIndex(bh)
	bt := &a.byType[bh.Type]
	if idx < len(bt.value) && idx != -1 {
		bt.isSet[idx] = false
	}
}

func (a *AssociatedData[T]) Len() int {
	count := 0
	a.For(func(_ restic.BlobHandle, _ T) {
		count++
	})
	return count
}

func (a *AssociatedData[T]) For(cb func(bh restic.BlobHandle, val T)) {
	for k, v := range a.overflow {
		cb(k, v)
	}

	a.idx.Each(context.TODO(), func(pb restic.PackedBlob) {
		val, known := a.Get(pb.BlobHandle)
		if known {
			cb(pb.BlobHandle, val)
		}
	})
}
