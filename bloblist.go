package khepri

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"
	"sync"
)

type BlobList struct {
	list []Blob
	m    sync.Mutex
}

var ErrBlobNotFound = errors.New("Blob not found")

func NewBlobList() *BlobList {
	return &BlobList{
		list: []Blob{},
	}
}

func (bl *BlobList) find(blob Blob) (int, Blob, error) {
	pos := sort.Search(len(bl.list), func(i int) bool {
		return blob.ID.Compare(bl.list[i].ID) >= 0
	})

	if pos < len(bl.list) && blob.ID.Compare(bl.list[pos].ID) == 0 {
		return pos, bl.list[pos], nil
	}

	return pos, Blob{}, ErrBlobNotFound
}

func (bl *BlobList) Find(blob Blob) (Blob, error) {
	bl.m.Lock()
	defer bl.m.Unlock()

	_, blob, err := bl.find(blob)
	return blob, err
}

func (bl *BlobList) Merge(other *BlobList) {
	bl.m.Lock()
	defer bl.m.Unlock()
	other.m.Lock()
	defer other.m.Unlock()

	for _, blob := range other.list {
		bl.insert(blob)
	}
}

func (bl *BlobList) insert(blob Blob) {
	pos, _, err := bl.find(blob)
	if err == nil {
		// already present
		return
	}

	// insert blob
	// https://code.google.com/p/go-wiki/wiki/bliceTricks
	bl.list = append(bl.list, Blob{})
	copy(bl.list[pos+1:], bl.list[pos:])
	bl.list[pos] = blob
}

func (bl *BlobList) Insert(blob Blob) {
	bl.m.Lock()
	defer bl.m.Unlock()

	bl.insert(blob)
}

func (bl BlobList) MarshalJSON() ([]byte, error) {
	return json.Marshal(bl.list)
}

func (bl *BlobList) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &bl.list)
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
