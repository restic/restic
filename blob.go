package restic

import (
	"fmt"

	"github.com/restic/restic/backend"
)

type Blob struct {
	ID          backend.ID `json:"id,omitempty"`
	Offset      uint64     `json:"offset,omitempty"`
	Size        uint64     `json:"size,omitempty"`
	Storage     backend.ID `json:"sid,omitempty"`   // encrypted ID
	StorageSize uint64     `json:"ssize,omitempty"` // encrypted Size
}

type Blobs []Blob

func (b Blob) Free() {
	if b.ID != nil {
		b.ID.Free()
	}

	if b.Storage != nil {
		b.Storage.Free()
	}
}

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
