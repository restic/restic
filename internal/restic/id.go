package restic

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/minio/sha256-simd"
)

// Hash returns the ID for data.
func Hash(data []byte) ID {
	return sha256.Sum256(data)
}

// idSize contains the size of an ID, in bytes.
const idSize = sha256.Size

// ID references content within a repository.
type ID [idSize]byte

// ParseID converts the given string to an ID.
func ParseID(s string) (ID, error) {
	if len(s) != hex.EncodedLen(idSize) {
		return ID{}, fmt.Errorf("invalid length for ID: %q", s)
	}

	b, err := hex.DecodeString(s)
	if err != nil {
		return ID{}, fmt.Errorf("invalid ID: %s", err)
	}

	id := ID{}
	copy(id[:], b)

	return id, nil
}

func (id ID) String() string {
	return hex.EncodeToString(id[:])
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

// MarshalJSON returns the JSON encoding of id.
func (id ID) MarshalJSON() ([]byte, error) {
	buf := make([]byte, 2+hex.EncodedLen(len(id)))

	buf[0] = '"'
	hex.Encode(buf[1:], id[:])
	buf[len(buf)-1] = '"'

	return buf, nil
}

// UnmarshalJSON parses the JSON-encoded data and stores the result in id.
func (id *ID) UnmarshalJSON(b []byte) error {
	// check string length
	if len(b) != len(`""`)+hex.EncodedLen(idSize) {
		return fmt.Errorf("invalid length for ID: %q", b)
	}

	if b[0] != '"' {
		return fmt.Errorf("invalid start of string: %q", b[0])
	}

	// Strip JSON string delimiters. The json.Unmarshaler contract says we get
	// a valid JSON value, so we don't need to check that b[len(b)-1] == '"'.
	b = b[1 : len(b)-1]

	_, err := hex.Decode(id[:], b)
	if err != nil {
		return fmt.Errorf("invalid ID: %s", err)
	}

	return nil
}

// IDFromHash returns the ID for the hash.
func IDFromHash(hash []byte) (id ID) {
	if len(hash) != idSize {
		panic("invalid hash type, not enough/too many bytes")
	}

	copy(id[:], hash)
	return id
}
