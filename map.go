package restic

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"
	"sync"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/debug"
)

type Map struct {
	list []Blob
	m    sync.Mutex
}

var ErrBlobNotFound = errors.New("Blob not found")

func NewMap() *Map {
	return &Map{
		list: []Blob{},
	}
}

func (bl *Map) find(blob Blob, checkSize bool) (int, Blob, error) {
	pos := sort.Search(len(bl.list), func(i int) bool {
		return blob.ID.Compare(bl.list[i].ID) >= 0
	})

	if pos < len(bl.list) {
		b := bl.list[pos]
		if blob.ID.Compare(b.ID) == 0 && (!checkSize || blob.Size == b.Size) {
			return pos, b, nil
		}
	}

	return pos, Blob{}, ErrBlobNotFound
}

func (bl *Map) Find(blob Blob) (Blob, error) {
	bl.m.Lock()
	defer bl.m.Unlock()

	_, blob, err := bl.find(blob, true)
	return blob, err
}

func (bl *Map) FindID(id backend.ID) (Blob, error) {
	bl.m.Lock()
	defer bl.m.Unlock()

	_, blob, err := bl.find(Blob{ID: id}, false)
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

func (bl *Map) insert(blob Blob) Blob {
	pos, b, err := bl.find(blob, true)
	if err == nil {
		// already present
		return b
	}

	// insert blob
	// https://code.google.com/p/go-wiki/wiki/SliceTricks
	bl.list = append(bl.list, Blob{})
	copy(bl.list[pos+1:], bl.list[pos:])
	bl.list[pos] = blob

	return blob
}

func (bl *Map) Insert(blob Blob) Blob {
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
	bl.m.Lock()
	defer bl.m.Unlock()

	if len(bl.list) != len(other.list) {
		return false
	}

	for i := 0; i < len(bl.list); i++ {
		if bl.list[i].Compare(other.list[i]) != 0 {
			return false
		}
	}

	return true
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

	pos, _, err := m.find(Blob{ID: id}, false)
	if err != nil {
		return
	}

	m.list = append(m.list[:pos], m.list[pos+1:]...)
}

// Compare compares two blobs by comparing the ID and the size. It returns -1,
// 0, or 1.
func (blob Blob) Compare(other Blob) int {
	if res := bytes.Compare(other.ID, blob.ID); res != 0 {
		return res
	}

	if blob.Size < other.Size {
		return -1
	}
	if blob.Size > other.Size {
		return 1
	}

	return 0
}
