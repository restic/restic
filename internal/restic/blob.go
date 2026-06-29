package restic

import (
	"fmt"

	"github.com/restic/restic/internal/errors"
)

// PackBlob is one index entry for a blob in a pack file.
// The interface intentionally omits the offset at which a blob is stored in the pack.
// This ensures that pack file internals are not leaked.
type PackBlob interface {
	PackID() ID
	Handle() BlobHandle
	// CiphertextLength is the encrypted size stored in the pack.
	CiphertextLength() uint
	// UncompressedCiphertextLength is the encrypted size of the uncompressed blob.
	UncompressedCiphertextLength() uint
	// PlaintextLength is the size after decryption/decompression.
	PlaintextLength() uint
	IsCompressed() bool
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
	NumBlobTypes // Number of types. Must be last in this enumeration.
)

func (t BlobType) String() string {
	switch t {
	case DataBlob:
		return "data"
	case TreeBlob:
		return "tree"
	case InvalidBlob:
		return "invalid"
	}

	return fmt.Sprintf("<BlobType %d>", t)
}

func (t BlobType) IsMetadata() bool {
	switch t {
	case TreeBlob:
		return true
	default:
		return false
	}
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

// BlobHandles is an ordered list of BlobHandles that implements sort.Interface.
type BlobHandles []BlobHandle

func (h BlobHandles) Len() int {
	return len(h)
}

func (h BlobHandles) Less(i, j int) bool {
	for k, b := range h[i].ID {
		if b == h[j].ID[k] {
			continue
		}

		return b < h[j].ID[k]
	}

	return h[i].Type < h[j].Type
}

func (h BlobHandles) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h BlobHandles) String() string {
	elements := make([]string, 0, len(h))
	for _, e := range h {
		elements = append(elements, e.String())
	}
	return fmt.Sprint(elements)
}
