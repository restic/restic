package rechunker

import (
	"maps"
	"sync"
)

type debugNoteType struct {
	d  map[string]int
	mu sync.Mutex
}

func newDebugNote(enable bool) *debugNoteType {
	if enable {
		return &debugNoteType{
			d: map[string]int{},
		}
	}
	return nil
}

func (n *debugNoteType) Add(k string, v int) {
	if n == nil {
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	n.d[k] += v
}

func (n *debugNoteType) AddMap(m map[string]int) {
	if n == nil {
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	for k, v := range m {
		n.d[k] += v
	}
}

func (n *debugNoteType) UpdateMax(k string, v int) {
	if n == nil {
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	if n.d[k] < v {
		n.d[k] = v
	}
}

func (n *debugNoteType) Dump() (note map[string]int) {
	if n == nil {
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	note = map[string]int{}
	maps.Copy(note, n.d)

	return note
}
