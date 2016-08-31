package restic

import (
	"errors"
	"fmt"
)

type Blob struct {
	ID          *ID    `json:"id,omitempty"`
	Size        uint64 `json:"size,omitempty"`
	Storage     *ID    `json:"sid,omitempty"`   // encrypted ID
	StorageSize uint64 `json:"ssize,omitempty"` // encrypted Size
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

// BlobHandle identifies a blob of a given type.
type BlobHandle struct {
	ID   ID
	Type BlobType
}

func (h BlobHandle) String() string {
	return fmt.Sprintf("<%s/%s>", h.Type, h.ID.Str())
}

// BlobType specifies what a blob stored in a pack is.
type BlobType uint8

// These are the blob types that can be stored in a pack.
const (
	InvalidBlob BlobType = iota
	DataBlob
	TreeBlob
)

func (t BlobType) String() string {
	switch t {
	case DataBlob:
		return "data"
	case TreeBlob:
		return "tree"
	}

	return fmt.Sprintf("<BlobType %d>", t)
}

// MarshalJSON encodes the BlobType into JSON.
func (t BlobType) MarshalJSON() ([]byte, error) {
	switch t {
	case DataBlob:
		return []byte(`"data"`), nil
	case TreeBlob:
		return []byte(`"tree"`), nil
	}

	return nil, errors.New("unknown blob type")
}

// UnmarshalJSON decodes the BlobType from JSON.
func (t *BlobType) UnmarshalJSON(buf []byte) error {
	switch string(buf) {
	case `"data"`:
		*t = DataBlob
	case `"tree"`:
		*t = TreeBlob
	default:
		return errors.New("unknown blob type")
	}

	return nil
}
