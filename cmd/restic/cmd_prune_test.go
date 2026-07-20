package main

import (
	"testing"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

// treeHandle returns a distinct tree-blob handle for the given seed.
func treeHandle(seed byte) restic.BlobHandle {
	var id restic.ID
	id[0] = seed
	return restic.BlobHandle{ID: id, Type: restic.TreeBlob}
}

// dataHandle returns a distinct data-blob handle for the given seed.
func dataHandle(seed byte) restic.BlobHandle {
	var id restic.ID
	id[0] = seed
	return restic.BlobHandle{ID: id, Type: restic.DataBlob}
}

// fakeBlobSet is a minimal restic.FindBlobSet backed by a map, used to verify
// that groupingSet forwards every insert to the underlying set.
type fakeBlobSet map[restic.BlobHandle]struct{}

func (s fakeBlobSet) Has(bh restic.BlobHandle) bool { _, ok := s[bh]; return ok }
func (s fakeBlobSet) Insert(bh restic.BlobHandle)   { s[bh] = struct{}{} }

func TestGroupingSet(t *testing.T) {
	inner := fakeBlobSet{}
	gs := &groupingSet{inner: inner, treeGroups: make(map[restic.BlobHandle]uint32)}

	tree := treeHandle(1)
	data := dataHandle(2)

	// group 1 reaches the tree blob and a data blob first
	gs.cur = 1
	gs.Insert(tree)
	gs.Insert(data)

	// group 2 reaches the same tree blob later; the first group must win
	gs.cur = 2
	gs.Insert(tree)

	// only tree blobs are recorded, and with the first group that reached them
	rtest.Equals(t, map[restic.BlobHandle]uint32{tree: 1}, gs.treeGroups)

	// every handle, tree or data, is forwarded to the underlying set
	rtest.Assert(t, inner.Has(tree), "tree blob not forwarded to inner set")
	rtest.Assert(t, inner.Has(data), "data blob not forwarded to inner set")

	// Has delegates to the underlying set
	rtest.Assert(t, gs.Has(tree), "Has should report a forwarded blob as present")
	rtest.Assert(t, !gs.Has(treeHandle(9)), "Has should report an unknown blob as absent")
}
