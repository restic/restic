//go:build darwin || freebsd || linux
// +build darwin freebsd linux

package fuse

import (
	"sync"

	"github.com/anacrolix/fuse/fs"
)

type treeCache struct {
	nodes map[string]fs.Node
	m     sync.Mutex
}

type forgetFn func()

func newTreeCache() *treeCache {
	return &treeCache{
		nodes: map[string]fs.Node{},
	}
}

func (t *treeCache) lookupOrCreate(name string, create func(forget forgetFn) (fs.Node, error)) (fs.Node, error) {
	t.m.Lock()
	defer t.m.Unlock()

	if node, ok := t.nodes[name]; ok {
		return node, nil
	}

	node, err := create(func() {
		t.m.Lock()
		defer t.m.Unlock()

		delete(t.nodes, name)
	})
	if err != nil {
		return nil, err
	}

	t.nodes[name] = node
	return node, nil
}
