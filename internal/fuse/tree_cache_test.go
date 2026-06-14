//go:build darwin || freebsd || linux

package fuse

import (
	"context"
	"testing"

	"github.com/anacrolix/fuse"
	"github.com/anacrolix/fuse/fs"

	"github.com/restic/restic/internal/test"
)

type cacheTestNode struct {
	id int
}

func (n cacheTestNode) Attr(context.Context, *fuse.Attr) error {
	return nil
}

func TestTreeCacheGeneration(t *testing.T) {
	cache := newTreeCache()
	created := 0
	create := func(forgetFn) (fs.Node, error) {
		created++
		return cacheTestNode{id: created}, nil
	}

	node1, err := cache.lookupOrCreate("node", 1, create)
	test.OK(t, err)
	node2, err := cache.lookupOrCreate("node", 1, create)
	test.OK(t, err)
	test.Assert(t, node1 == node2, "lookup should reuse cached node")
	test.Equals(t, 1, created)

	node3, err := cache.lookupOrCreate("node", 2, create)
	test.OK(t, err)
	test.Assert(t, node1 != node3, "lookup should recreate node after generation change")
	test.Equals(t, 2, created)

	node4, err := cache.lookupOrCreate("node", -1, create)
	test.OK(t, err)
	test.Assert(t, node3 == node4, "negative generation should not reset cache")
	test.Equals(t, 2, created)

	node5, err := cache.lookupOrCreate("node", 3, create)
	test.OK(t, err)
	test.Assert(t, node4 != node5, "lookup should still track later generation changes")
	test.Equals(t, 3, created)
}
