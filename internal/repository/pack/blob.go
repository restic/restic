package pack

import (
	"fmt"

	"github.com/restic/restic/internal/repository/crypto"
	"github.com/restic/restic/internal/restic"
)

// Blob is one part of a file or a tree with pack layout information.
type Blob struct {
	restic.BlobHandle
	Length             uint
	Offset             uint
	UncompressedLength uint
}

func (b Blob) String() string {
	return fmt.Sprintf("<Blob (%v) %v, offset %v, length %v, uncompressed length %v>",
		b.Type, b.ID.Str(), b.Offset, b.Length, b.UncompressedLength)
}

func (b Blob) DataLength() uint {
	if b.UncompressedLength != 0 {
		return b.UncompressedLength
	}
	return uint(crypto.PlaintextLength(int(b.Length)))
}

func (b Blob) UncompressedCiphertextLength() uint {
	return uint(crypto.CiphertextLength(int(b.DataLength())))
}

func (b Blob) IsCompressed() bool {
	return b.UncompressedLength != 0
}
