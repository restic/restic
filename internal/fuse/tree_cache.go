//go:build darwin || freebsd || linux || windows
// +build darwin freebsd linux windows

package fuse

import (
	"sync"
)

type treeCache struct {
	nodes map[string]Node
	m     sync.Mutex
}

type forgetFn func()

func newTreeCache() *treeCache {
	return &treeCache{
		nodes: map[string]Node{},
	}
}

func (t *treeCache) lookupOrCreate(name string, create func(forget forgetFn) (Node, error)) (Node, error) {
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
