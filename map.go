package restic

import (
	"encoding/json"
	"errors"
	"sort"
	"sync"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/debug"
	"github.com/restic/restic/server"
)

type Map struct {
	list []server.Blob
	m    sync.Mutex
}

var ErrBlobNotFound = errors.New("Blob not found")

func NewMap() *Map {
	return &Map{
		list: []server.Blob{},
	}
}

func (bl *Map) find(blob server.Blob, checkSize bool) (int, server.Blob, error) {
	pos := sort.Search(len(bl.list), func(i int) bool {
		return blob.ID.Compare(bl.list[i].ID) >= 0
	})

	if pos < len(bl.list) {
		b := bl.list[pos]
		if blob.ID.Compare(b.ID) == 0 && (!checkSize || blob.Size == b.Size) {
			return pos, b, nil
		}
	}

	return pos, server.Blob{}, ErrBlobNotFound
}

func (bl *Map) Find(blob server.Blob) (server.Blob, error) {
	bl.m.Lock()
	defer bl.m.Unlock()

	_, blob, err := bl.find(blob, true)
	return blob, err
}

func (bl *Map) FindID(id backend.ID) (server.Blob, error) {
	bl.m.Lock()
	defer bl.m.Unlock()

	_, blob, err := bl.find(server.Blob{ID: id}, false)
	return blob, err
}

func (bl *Map) Merge(other *Map) {
	bl.m.Lock()
	defer bl.m.Unlock()
	other.m.Lock()
	defer other.m.Unlock()

	for _, blob := range other.list {
		bl.insert(blob)
	}
}

func (bl *Map) insert(blob server.Blob) server.Blob {
	pos, b, err := bl.find(blob, true)
	if err == nil {
		// already present
		return b
	}

	// insert blob
	// https://code.google.com/p/go-wiki/wiki/SliceTricks
	bl.list = append(bl.list, server.Blob{})
	copy(bl.list[pos+1:], bl.list[pos:])
	bl.list[pos] = blob

	return blob
}

func (bl *Map) Insert(blob server.Blob) server.Blob {
	bl.m.Lock()
	defer bl.m.Unlock()

	debug.Log("Map.Insert", "  Map<%p> insert %v", bl, blob)

	return bl.insert(blob)
}

func (bl *Map) MarshalJSON() ([]byte, error) {
	return json.Marshal(bl.list)
}

func (bl *Map) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &bl.list)
}

func (bl *Map) IDs() []backend.ID {
	bl.m.Lock()
	defer bl.m.Unlock()

	ids := make([]backend.ID, 0, len(bl.list))
	for _, b := range bl.list {
		ids = append(ids, b.ID)
	}

	return ids
}

func (bl *Map) StorageIDs() []backend.ID {
	bl.m.Lock()
	defer bl.m.Unlock()

	ids := make([]backend.ID, 0, len(bl.list))
	for _, b := range bl.list {
		ids = append(ids, b.Storage)
	}

	return ids
}

func (bl *Map) Equals(other *Map) bool {
	if bl == nil && other == nil {
		return true
	}

	if bl == nil || other == nil {
		return false
	}

	bl.m.Lock()
	defer bl.m.Unlock()

	if len(bl.list) != len(other.list) {
		debug.Log("Map.Equals", "length does not match: %d != %d", len(bl.list), len(other.list))
		return false
	}

	for i := 0; i < len(bl.list); i++ {
		if bl.list[i].Compare(other.list[i]) != 0 {
			debug.Log("Map.Equals", "entry %d does not match: %v != %v", i, bl.list[i], other.list[i])
			return false
		}
	}

	return true
}

// Each calls f for each blob in the Map. While Each is running, no other
// operation is possible, since a mutex is held for the whole time.
func (bl *Map) Each(f func(blob server.Blob)) {
	bl.m.Lock()
	defer bl.m.Unlock()

	for _, blob := range bl.list {
		f(blob)
	}
}

// Select returns a list of of blobs from the plaintext IDs given in list.
func (bl *Map) Select(list backend.IDs) (server.Blobs, error) {
	bl.m.Lock()
	defer bl.m.Unlock()

	blobs := make(server.Blobs, 0, len(list))
	for _, id := range list {
		_, blob, err := bl.find(server.Blob{ID: id}, false)
		if err != nil {
			return nil, err
		}

		blobs = append(blobs, blob)
	}

	return blobs, nil
}

// Len returns the number of blobs in the map.
func (bl *Map) Len() int {
	bl.m.Lock()
	defer bl.m.Unlock()

	return len(bl.list)
}

// Prune deletes all IDs from the map except the ones listed in ids.
func (m *Map) Prune(ids *backend.IDSet) {
	m.m.Lock()
	defer m.m.Unlock()

	pos := 0
	for pos < len(m.list) {
		blob := m.list[pos]
		if ids.Find(blob.ID) != nil {
			// remove element
			m.list = append(m.list[:pos], m.list[pos+1:]...)
			continue
		}

		pos++
	}
}

// DeleteID removes the plaintext ID id from the map.
func (m *Map) DeleteID(id backend.ID) {
	m.m.Lock()
	defer m.m.Unlock()

	pos, _, err := m.find(server.Blob{ID: id}, false)
	if err != nil {
		return
	}

	m.list = append(m.list[:pos], m.list[pos+1:]...)
}
