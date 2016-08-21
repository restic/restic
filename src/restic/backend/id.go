package backend

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/pkg/errors"
)

// Hash returns the ID for data.
func Hash(data []byte) ID {
	return sha256.Sum256(data)
}

// IDSize contains the size of an ID, in bytes.
const IDSize = sha256.Size

// ID references content within a repository.
type ID [IDSize]byte

// ParseID converts the given string to an ID.
func ParseID(s string) (ID, error) {
	b, err := hex.DecodeString(s)

	if err != nil {
		return ID{}, err
	}

	if len(b) != IDSize {
		return ID{}, errors.New("invalid length for hash")
	}

	id := ID{}
	copy(id[:], b)

	return id, nil
}

func (id ID) String() string {
	return hex.EncodeToString(id[:])
}

const shortStr = 4

// Str returns the shortened string version of id.
func (id *ID) Str() string {
	if id == nil {
		return "[nil]"
	}

	if id.IsNull() {
		return "[null]"
	}

	return hex.EncodeToString(id[:shortStr])
}

// IsNull returns true iff id only consists of null bytes.
func (id ID) IsNull() bool {
	var nullID ID

	return id == nullID
}

// Equal compares an ID to another other.
func (id ID) Equal(other ID) bool {
	return id == other
}

// EqualString compares this ID to another one, given as a string.
func (id ID) EqualString(other string) (bool, error) {
	s, err := hex.DecodeString(other)
	if err != nil {
		return false, err
	}

	id2 := ID{}
	copy(id2[:], s)

	return id == id2, nil
}

// Compare compares this ID to another one, returning -1, 0, or 1.
func (id ID) Compare(other ID) int {
	return bytes.Compare(other[:], id[:])
}

// MarshalJSON returns the JSON encoding of id.
func (id ID) MarshalJSON() ([]byte, error) {
	return json.Marshal(id.String())
}

// UnmarshalJSON parses the JSON-encoded data and stores the result in id.
func (id *ID) UnmarshalJSON(b []byte) error {
	var s string
	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}

	_, err = hex.Decode(id[:], []byte(s))
	if err != nil {
		return err
	}

	return nil
}
