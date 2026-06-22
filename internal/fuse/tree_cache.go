//go:build darwin || freebsd || linux

package fuse

import (
	"sync"

	"github.com/restic/restic/internal/debug"

	"github.com/anacrolix/fuse/fs"
)

type treeCache struct {
	nodes      map[string]fs.Node
	m          sync.Mutex
	generation int64
}

type forgetFn func()

func newTreeCache() *treeCache {
	return &treeCache{
		nodes: map[string]fs.Node{},
	}
}

func (t *treeCache) lookupOrCreate(name string, generation int64, create func(forget forgetFn) (fs.Node, error)) (fs.Node, error) {
	t.m.Lock()
	defer t.m.Unlock()

	if generation >= 0 && generation != t.generation {
		debug.Log("treeCache generation changed %d -> %d, resetting cache", t.generation, generation)
		t.nodes = make(map[string]fs.Node)
		t.generation = generation
	}

	if node, ok := t.nodes[name]; ok {
		return node, nil
	}

	cacheGeneration := t.generation
	node, err := create(func() {
		t.m.Lock()
		defer t.m.Unlock()

		if t.generation == cacheGeneration {
			delete(t.nodes, name)
		}
	})
	if err != nil {
		return nil, err
	}

	t.nodes[name] = node
	return node, nil
}
