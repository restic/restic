package storage

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
)

// References content within a repository.
type ID []byte

// ParseID converts the given string to an ID.
func ParseID(s string) (ID, error) {
	b, err := hex.DecodeString(s)

	if err != nil {
		return nil, err
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
