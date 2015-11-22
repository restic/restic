package backend

import (
	"bytes"
	"errors"
	"io"
	"sort"
	"sync"

	"github.com/restic/restic/debug"
)

type entry struct {
	Type Type
	Name string
}

type memMap map[entry][]byte

// MemoryBackend is a mock backend that uses a map for storing all data in
// memory. This should only be used for tests.
type MemoryBackend struct {
	data memMap
	m    sync.Mutex

	MockBackend
}

// NewMemoryBackend returns a new backend that saves all data in a map in
// memory.
func NewMemoryBackend() *MemoryBackend {
	be := &MemoryBackend{
		data: make(memMap),
	}

	be.MockBackend.TestFn = func(t Type, name string) (bool, error) {
		return memTest(be, t, name)
	}

	be.MockBackend.CreateFn = func() (Blob, error) {
		return memCreate(be)
	}

	be.MockBackend.GetFn = func(t Type, name string) (io.ReadCloser, error) {
		return memGet(be, t, name)
	}

	be.MockBackend.GetReaderFn = func(t Type, name string, offset, length uint) (io.ReadCloser, error) {
		return memGetReader(be, t, name, offset, length)
	}

	be.MockBackend.RemoveFn = func(t Type, name string) error {
		return memRemove(be, t, name)
	}

	be.MockBackend.ListFn = func(t Type, done <-chan struct{}) <-chan string {
		return memList(be, t, done)
	}

	debug.Log("MemoryBackend.New", "created new memory backend")

	return be
}

func (be *MemoryBackend) insert(t Type, name string, data []byte) error {
	be.m.Lock()
	defer be.m.Unlock()

	if _, ok := be.data[entry{t, name}]; ok {
		return errors.New("already present")
	}

	be.data[entry{t, name}] = data
	return nil
}

func memTest(be *MemoryBackend, t Type, name string) (bool, error) {
	be.m.Lock()
	defer be.m.Unlock()

	debug.Log("MemoryBackend.Test", "test %v %v", t, name)

	if _, ok := be.data[entry{t, name}]; ok {
		return true, nil
	}

	return false, nil
}

// tempMemEntry temporarily holds data written to the memory backend before it
// is finalized.
type tempMemEntry struct {
	be   *MemoryBackend
	data bytes.Buffer
}

func (e *tempMemEntry) Write(p []byte) (int, error) {
	return e.data.Write(p)
}

func (e *tempMemEntry) Size() uint {
	return uint(len(e.data.Bytes()))
}

func (e *tempMemEntry) Finalize(t Type, name string) error {
	if t == Config {
		name = ""
	}

	debug.Log("MemoryBackend", "save blob %p as %v %v", e, t, name)
	return e.be.insert(t, name, e.data.Bytes())
}

func memCreate(be *MemoryBackend) (Blob, error) {
	blob := &tempMemEntry{be: be}
	debug.Log("MemoryBackend.Create", "create new blob %p", blob)
	return blob, nil
}

// readCloser wraps a reader and adds a noop Close method.
type readCloser struct {
	io.Reader
}

func (rd readCloser) Close() error {
	return nil
}

func memGet(be *MemoryBackend, t Type, name string) (io.ReadCloser, error) {
	be.m.Lock()
	defer be.m.Unlock()

	if t == Config {
		name = ""
	}

	debug.Log("MemoryBackend.Get", "get %v %v", t, name)

	if _, ok := be.data[entry{t, name}]; !ok {
		return nil, errors.New("no such data")
	}

	return readCloser{bytes.NewReader(be.data[entry{t, name}])}, nil
}

func memGetReader(be *MemoryBackend, t Type, name string, offset, length uint) (io.ReadCloser, error) {
	be.m.Lock()
	defer be.m.Unlock()

	if t == Config {
		name = ""
	}

	debug.Log("MemoryBackend.GetReader", "get %v %v offset %v len %v", t, name, offset, length)

	if _, ok := be.data[entry{t, name}]; !ok {
		return nil, errors.New("no such data")
	}

	buf := be.data[entry{t, name}]
	if offset > uint(len(buf)) {
		return nil, errors.New("offset beyond end of file")
	}

	buf = buf[offset:]

	if length > 0 {
		if length > uint(len(buf)) {
			length = uint(len(buf))
		}

		buf = buf[:length]
	}

	return readCloser{bytes.NewReader(buf)}, nil
}

func memRemove(be *MemoryBackend, t Type, name string) error {
	be.m.Lock()
	defer be.m.Unlock()

	debug.Log("MemoryBackend.Remove", "get %v %v", t, name)

	if _, ok := be.data[entry{t, name}]; !ok {
		return errors.New("no such data")
	}

	delete(be.data, entry{t, name})

	return nil
}

func memList(be *MemoryBackend, t Type, done <-chan struct{}) <-chan string {
	be.m.Lock()
	defer be.m.Unlock()

	ch := make(chan string)

	var ids []string
	for entry := range be.data {
		if entry.Type != t {
			continue
		}
		ids = append(ids, entry.Name)
	}

	sort.Strings(ids)

	debug.Log("MemoryBackend.List", "list %v: %v", t, ids)

	go func() {
		defer close(ch)
		for _, id := range ids {
			select {
			case ch <- id:
			case <-done:
				return
			}
		}
	}()

	return ch
}
