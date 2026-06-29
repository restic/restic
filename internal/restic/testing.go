package restic

import (
	"crypto/rand"
	"fmt"
	"io"
)

// TestParseID parses s as a ID and panics if that fails.
func TestParseID(s string) ID {
	id, err := ParseID(s)
	if err != nil {
		panic(fmt.Sprintf("unable to parse string %q as ID: %v", s, err))
	}

	return id
}

// TestParseHandle parses s as a ID, panics if that fails and creates a BlobHandle with t.
func TestParseHandle(s string, t BlobType) BlobHandle {
	return BlobHandle{ID: TestParseID(s), Type: t}
}

func NewRandomBlobHandle() BlobHandle {
	return BlobHandle{ID: NewRandomID(), Type: DataBlob}
}

// NewRandomID returns a randomly generated ID. When reading from rand fails,
// the function panics.
func NewRandomID() ID {
	id := ID{}
	_, err := io.ReadFull(rand.Reader, id[:])
	if err != nil {
		panic(err)
	}
	return id
}
