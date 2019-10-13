package restic

import (
	"fmt"

	"github.com/restic/restic/internal/errors"
)

const (
	CompressionTypeStored uint8 = iota
	CompressionTypeZlib
)

// Blob is one part of a file or a tree.
type Blob struct {
	Type            BlobType
	ActualLength    uint  // How long the unpacked blob is
	PackedLength    uint  // How long the blob is in the pack.
	CompressionType uint8 // One of CompressionType*
	ID              ID
	Offset          uint
}

func (b Blob) String() string {
	return fmt.Sprintf("<Blob (%v) %v, offset %v, length %v (%v), comp %v>",
		b.Type, b.ID.Str(), b.Offset, b.ActualLength, b.PackedLength,
		b.CompressionType)
}

// Convert from the internal Blob struct to a form serializable as
// JSON.
func (b Blob) ToBlobJSON() BlobJSON {
	return BlobJSON{
		ID:              b.ID,
		Type:            b.Type,
		Offset:          b.Offset,
		ActualLength:    b.ActualLength,
		PackedLength:    b.PackedLength,
		CompressionType: b.CompressionType,
	}
}

// The serialized blob in the index. We include extra fields to
// support older versions of the index.
type BlobJSON struct {
	ID     ID       `json:"id"`
	Type   BlobType `json:"type"`
	Offset uint     `json:"offset"`

	// Legacy version only supports uncompressed length
	Length uint `json:"length"`

	// New index version
	ActualLength    uint  `json:"actual_length"`
	PackedLength    uint  `json:"packed_length"`
	CompressionType uint8 `json:"compression_type"`
}

// Take care of parsing older versions of the index.
func (blob BlobJSON) ToBlob() Blob {
	result := Blob{
		Type:   blob.Type,
		ID:     blob.ID,
		Offset: blob.Offset,
	}

	// Legacy index entry.
	if blob.Length > 0 {
		result.ActualLength = blob.Length
		result.PackedLength = blob.Length

	} else {
		result.ActualLength = blob.ActualLength
		result.PackedLength = blob.PackedLength
		result.CompressionType = blob.CompressionType
	}

	return result
}

// PackedBlob is a blob stored within a file.
type PackedBlob struct {
	Blob
	PackID ID
}

// BlobHandle identifies a blob of a given type. A BlobHandle is used
// as a key in indexes.
type BlobHandle struct {
	ID   ID
	Type BlobType
}

func (h BlobHandle) String() string {
	return fmt.Sprintf("<%s/%s>", h.Type, h.ID.Str())
}

// Create a normalize blob handle. We treat DataBlob as functionally
// equivalent to ZlibBlob.
func NewBlobHandle(id ID, t BlobType) BlobHandle {
	if t == ZlibBlob {
		t = DataBlob
	}

	return BlobHandle{id, t}
}

// BlobType specifies what a blob stored in a pack is.
type BlobType uint8

// These are the blob types that can be stored in a pack.
const (
	InvalidBlob BlobType = iota
	DataBlob
	TreeBlob

	// A data blob compressed with zlib.
	ZlibBlob
)

func (t BlobType) String() string {
	switch t {
	case ZlibBlob:
		return "zlib"
	case DataBlob:
		return "data"
	case TreeBlob:
		return "tree"
	case InvalidBlob:
		return "invalid"
	}

	return fmt.Sprintf("<BlobType %d>", t)
}

// MarshalJSON encodes the BlobType into JSON.
func (t BlobType) MarshalJSON() ([]byte, error) {
	switch t {
	case ZlibBlob:
		return []byte(`"zlib"`), nil
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
	case `"zlib"`:
		*t = ZlibBlob
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

		if b < h[j].ID[k] {
			return true
		}

		return false
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
	return fmt.Sprintf("%v", elements)
}
