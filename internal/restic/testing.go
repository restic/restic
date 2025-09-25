package restic

import (
	"fmt"
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
