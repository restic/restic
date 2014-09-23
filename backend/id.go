package backend

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
)

const sha256_length = 32 // in bytes

// References content within a repository.
type ID []byte

// ParseID converts the given string to an ID.
func ParseID(s string) (ID, error) {
	b, err := hex.DecodeString(s)

	if err != nil {
		return nil, err
	}

	if len(b) != sha256_length {
		return nil, errors.New("invalid length for sha256 hash")
	}

	return ID(b), nil
}

func (id ID) String() string {
	return hex.EncodeToString(id)
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

func (id ID) MarshalJSON() ([]byte, error) {
	return json.Marshal(id.String())
}

func (id *ID) UnmarshalJSON(b []byte) error {
	var s string
	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}

	*id = make([]byte, len(s)/2)
	_, err = hex.Decode(*id, []byte(s))
	if err != nil {
		return err
	}

	return nil
}

func IDFromData(d []byte) ID {
	hash := sha256.Sum256(d)
	id := make([]byte, sha256_length)
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
