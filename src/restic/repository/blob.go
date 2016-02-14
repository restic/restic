package repository

import (
	"fmt"

	"restic/backend"
)

type Blob struct {
	ID          *backend.ID `json:"id,omitempty"`
	Size        uint64      `json:"size,omitempty"`
	Storage     *backend.ID `json:"sid,omitempty"`   // encrypted ID
	StorageSize uint64      `json:"ssize,omitempty"` // encrypted Size
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
func (b Blob) Compare(other Blob) int {
	if res := b.ID.Compare(*other.ID); res != 0 {
		return res
	}

	if b.Size < other.Size {
		return -1
	}
	if b.Size > other.Size {
		return 1
	}

	return 0
}
