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

// buildTreeGroups assigns `count` distinct tree blobs to each listed group id.
// The seed keeps handles unique across groups.
func buildTreeGroups(perGroup map[uint32]int) map[restic.BlobHandle]uint32 {
	tg := make(map[restic.BlobHandle]uint32)
	seed := byte(0)
	for g, n := range perGroup {
		for i := 0; i < n; i++ {
			tg[treeHandle(seed)] = g
			seed++
		}
	}
	return tg
}

func TestDemoteSmallGroups(t *testing.T) {
	// four groups with 1/3/2/3 tree blobs; keep the two largest. Ranking is by
	// count, tie broken by ascending id, so groups 2 and 4 (both count 3) win
	// and are remapped to the dense range 1..2; groups 1 and 3 are dropped so
	// their blobs fall back to the shared bucket.
	tg := buildTreeGroups(map[uint32]int{1: 1, 2: 3, 3: 2, 4: 3})
	demoteSmallGroups(tg, 2)

	counts := make(map[uint32]int)
	for _, g := range tg {
		counts[g]++
	}

	// only the dense ids 1 and 2 must survive, with the original counts (3 each)
	rtest.Equals(t, map[uint32]int{1: 3, 2: 3}, counts)
}

func TestDemoteSmallGroupsNoOp(t *testing.T) {
	// budget >= number of groups: nothing is dropped, but ids are still
	// remapped to a dense 1..K range (here already dense, so unchanged counts).
	tg := buildTreeGroups(map[uint32]int{1: 2, 2: 1, 3: 4})
	demoteSmallGroups(tg, 3)

	counts := make(map[uint32]int)
	for _, g := range tg {
		counts[g]++
	}
	rtest.Equals(t, map[uint32]int{1: 4, 2: 2, 3: 1}, counts)
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
