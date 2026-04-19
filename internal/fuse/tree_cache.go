//go:build darwin || freebsd || linux

package fuse

import (
	"sync"

	"github.com/anacrolix/fuse/fs"
)

type treeCache struct {
	nodes      map[string]fs.Node
	generation uint64
	m          sync.Mutex
}

type forgetFn func()

func newTreeCache() *treeCache {
	return &treeCache{
		nodes: map[string]fs.Node{},
	}
}

func (t *treeCache) lookupOrCreate(name string, create func(forget forgetFn) (fs.Node, error)) (fs.Node, error) {
	return t.lookupOrCreateAtGeneration(0, name, create)
}

func (t *treeCache) lookupOrCreateAtGeneration(generation uint64, name string, create func(forget forgetFn) (fs.Node, error)) (fs.Node, error) {
	t.m.Lock()
	defer t.m.Unlock()

	if t.generation != generation {
		t.nodes = map[string]fs.Node{}
		t.generation = generation
	}

	if node, ok := t.nodes[name]; ok {
		return node, nil
	}

	cacheGeneration := generation
	node, err := create(func() {
		t.m.Lock()
		defer t.m.Unlock()

		if t.generation != cacheGeneration {
			return
		}
		delete(t.nodes, name)
	})
	if err != nil {
		return nil, err
	}

	t.nodes[name] = node
	return node, nil
}
