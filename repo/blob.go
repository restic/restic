package repo

import (
	"bytes"
	"fmt"

	"github.com/restic/restic/backend"
)

type Blob struct {
	ID          backend.ID `json:"id,omitempty"`
	Size        uint64     `json:"size,omitempty"`
	Storage     backend.ID `json:"sid,omitempty"`   // encrypted ID
	StorageSize uint64     `json:"ssize,omitempty"` // encrypted Size
}

type Blobs []Blob

func (b Blob) Valid() bool {
	if b.ID == nil || b.Storage == nil || b.StorageSize == 0 {
		return false
	}

	return true
}

func (b Blob) String() string {
	return fmt.Sprintf("Blob<%s (%d) -> %s (%d)>",
		b.ID.Str(), b.Size,
		b.Storage.Str(), b.StorageSize)
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
