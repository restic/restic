package backend

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
)

// IDSize contains the size of an ID, in bytes.
const IDSize = hashSize

// References content within a repository.
type ID []byte

// ParseID converts the given string to an ID.
func ParseID(s string) (ID, error) {
	b, err := hex.DecodeString(s)

	if err != nil {
		return nil, err
	}

	if len(b) != IDSize {
		return nil, errors.New("invalid length for hash")
	}

	return ID(b), nil
}

func (id ID) String() string {
	return hex.EncodeToString(id)
}

const shortStr = 4

func (id ID) Str() string {
	if id == nil {
		return "[nil]"
	}
	return hex.EncodeToString(id[:shortStr])
}

// Equal compares an ID to another other.
func (id ID) Equal(other ID) bool {
	return bytes.Equal(id, other)
}

// EqualString compares this ID to another one, given as a string.
func (id ID) EqualString(other string) (bool, error) {
	s, err := hex.DecodeString(other)
	if err != nil {
		return false, err
	}

	return id.Equal(ID(s)), nil
}

// Compare compares this ID to another one, returning -1, 0, or 1.
func (id ID) Compare(other ID) int {
	return bytes.Compare(other, id)
}

func (id ID) MarshalJSON() ([]byte, error) {
	return json.Marshal(id.String())
}

func (id *ID) UnmarshalJSON(b []byte) error {
	var s string
	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}

	*id = make([]byte, IDSize)
	_, err = hex.Decode(*id, []byte(s))
	if err != nil {
		return err
	}

	return nil
}

func IDFromData(d []byte) ID {
	hash := hashData(d)
	id := make([]byte, IDSize)
	copy(id, hash[:])
	return id
}

type IDs []ID

func (ids IDs) Len() int {
	return len(ids)
}

func (ids IDs) Less(i, j int) bool {
	if len(ids[i]) < len(ids[j]) {
		return true
	}

	for k, b := range ids[i] {
		if b == ids[j][k] {
			continue
		}

		if b < ids[j][k] {
			return true
		} else {
			return false
		}
	}

	return false
}

func (ids IDs) Swap(i, j int) {
	ids[i], ids[j] = ids[j], ids[i]
}
