package index

import (
	"context"

	"github.com/restic/restic/internal/restic"
)

type associatedDataSub struct {
	value []uint8
	isSet []bool
}

type AssociatedData struct {
	byType   [restic.NumBlobTypes]associatedDataSub
	overflow map[restic.BlobHandle]uint8
	idx      *MasterIndex
}

func NewAssociated(mi *MasterIndex) *AssociatedData {
	a := AssociatedData{
		overflow: make(map[restic.BlobHandle]uint8),
		idx:      mi,
	}

	for typ := range a.byType {
		if typ == 0 {
			continue
		}
		count := mi.len(restic.BlobType(typ))
		a.byType[typ].value = make([]uint8, count)
		a.byType[typ].isSet = make([]bool, count)
	}

	return &a
}

func (a *AssociatedData) Get(bh restic.BlobHandle) (uint8, bool) {
	if val, ok := a.overflow[bh]; ok {
		return val, true
	}

	idx := a.idx.getBlobIndex(bh)
	bt := &a.byType[bh.Type]
	if idx >= len(bt.value) || idx == -1 {
		return 0, false
	}

	has := bt.isSet[idx]
	if has {
		return bt.value[idx], has
	} else {
		return 0, false
	}
}

func (a *AssociatedData) Has(bh restic.BlobHandle) bool {
	_, ok := a.Get(bh)
	return ok
}

func (a *AssociatedData) Set(bh restic.BlobHandle, val uint8) {
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

func (a *AssociatedData) Insert(bh restic.BlobHandle) {
	a.Set(bh, 0)
}

func (a *AssociatedData) Delete(bh restic.BlobHandle) {
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

func (a *AssociatedData) Len() int {
	count := 0
	a.For(func(_ restic.BlobHandle, _ uint8) {
		count++
	})
	return count
}

func (a *AssociatedData) For(cb func(bh restic.BlobHandle, val uint8)) {
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
