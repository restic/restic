package mem

import (
	"io"
	"restic"
	"sync"

	"restic/errors"

	"restic/debug"
)

type entry struct {
	Type restic.FileType
	Name string
}

type memMap map[entry][]byte

// make sure that MemoryBackend implements backend.Backend
var _ restic.Backend = &MemoryBackend{}

// MemoryBackend is a mock backend that uses a map for storing all data in
// memory. This should only be used for tests.
type MemoryBackend struct {
	data memMap
	m    sync.Mutex
}

// New returns a new backend that saves all data in a map in memory.
func New() *MemoryBackend {
	be := &MemoryBackend{
		data: make(memMap),
	}

	debug.Log("created new memory backend")

	return be
}

// Test returns whether a file exists.
func (be *MemoryBackend) Test(t restic.FileType, name string) (bool, error) {
	be.m.Lock()
	defer be.m.Unlock()

	debug.Log("test %v %v", t, name)

	if _, ok := be.data[entry{t, name}]; ok {
		return true, nil
	}

	return false, nil
}

// Load reads data from the backend.
func (be *MemoryBackend) Load(h restic.Handle, p []byte, off int64) (int, error) {
	if err := h.Valid(); err != nil {
		return 0, err
	}

	be.m.Lock()
	defer be.m.Unlock()

	if h.Type == restic.ConfigFile {
		h.Name = ""
	}

	debug.Log("get %v offset %v len %v", h, off, len(p))

	if _, ok := be.data[entry{h.Type, h.Name}]; !ok {
		return 0, errors.New("no such data")
	}

	buf := be.data[entry{h.Type, h.Name}]
	switch {
	case off > int64(len(buf)):
		return 0, errors.New("offset beyond end of file")
	case off < -int64(len(buf)):
		off = 0
	case off < 0:
		off = int64(len(buf)) + off
	}

	buf = buf[off:]

	n := copy(p, buf)

	if len(p) > len(buf) {
		return n, io.ErrUnexpectedEOF
	}

	return n, nil
}

// Save adds new Data to the backend.
func (be *MemoryBackend) Save(h restic.Handle, p []byte) error {
	if err := h.Valid(); err != nil {
		return err
	}

	be.m.Lock()
	defer be.m.Unlock()

	if h.Type == restic.ConfigFile {
		h.Name = ""
	}

	if _, ok := be.data[entry{h.Type, h.Name}]; ok {
		return errors.New("file already exists")
	}

	debug.Log("save %v bytes at %v", len(p), h)
	buf := make([]byte, len(p))
	copy(buf, p)
	be.data[entry{h.Type, h.Name}] = buf

	return nil
}

// Stat returns information about a file in the backend.
func (be *MemoryBackend) Stat(h restic.Handle) (restic.FileInfo, error) {
	be.m.Lock()
	defer be.m.Unlock()

	if err := h.Valid(); err != nil {
		return restic.FileInfo{}, err
	}

	if h.Type == restic.ConfigFile {
		h.Name = ""
	}

	debug.Log("stat %v", h)

	e, ok := be.data[entry{h.Type, h.Name}]
	if !ok {
		return restic.FileInfo{}, errors.New("no such data")
	}

	return restic.FileInfo{Size: int64(len(e))}, nil
}

// Remove deletes a file from the backend.
func (be *MemoryBackend) Remove(t restic.FileType, name string) error {
	be.m.Lock()
	defer be.m.Unlock()

	debug.Log("get %v %v", t, name)

	if _, ok := be.data[entry{t, name}]; !ok {
		return errors.New("no such data")
	}

	delete(be.data, entry{t, name})

	return nil
}

// List returns a channel which yields entries from the backend.
func (be *MemoryBackend) List(t restic.FileType, done <-chan struct{}) <-chan string {
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

	debug.Log("list %v: %v", t, ids)

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

// Location returns the location of the backend (RAM).
func (be *MemoryBackend) Location() string {
	return "RAM"
}

// Delete removes all data in the backend.
func (be *MemoryBackend) Delete() error {
	be.m.Lock()
	defer be.m.Unlock()

	be.data = make(memMap)
	return nil
}

// Close closes the backend.
func (be *MemoryBackend) Close() error {
	return nil
}
