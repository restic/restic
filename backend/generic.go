package backend

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
)

const (
	MinPrefixLength = 4
)

var (
	ErrNoIDPrefixFound   = errors.New("no ID found")
	ErrMultipleIDMatches = errors.New("multiple IDs with prefix found")
)

var (
	newHash  = sha256.New
	hashData = sha256.Sum256
)

const hashSize = sha256.Size

// Each lists all entries of type t in the backend and calls function f() with
// the id and data.
func Each(be interface {
	Lister
	Getter
}, t Type, f func(id ID, data []byte, err error)) error {
	ids, err := be.List(t)
	if err != nil {
		return err
	}

	for _, id := range ids {
		data, err := be.Get(t, id)
		if err != nil {
			f(id, nil, err)
			continue
		}

		f(id, data, nil)
	}

	return nil
}

// Each lists all entries of type t in the backend and calls function f() with
// the id.
func EachID(be Lister, t Type, f func(ID)) error {
	ids, err := be.List(t)
	if err != nil {
		return err
	}

	for _, id := range ids {
		f(id)
	}

	return nil
}

// Hash returns the ID for data.
func Hash(data []byte) ID {
	h := hashData(data)
	id := idPool.Get().(ID)
	copy(id, h[:])
	return id
}

// Find loads the list of all blobs of type t and searches for IDs which start
// with prefix. If none is found, nil and ErrNoIDPrefixFound is returned. If
// more than one is found, nil and ErrMultipleIDMatches is returned.
func Find(be Lister, t Type, prefix string) (ID, error) {
	p, err := hex.DecodeString(prefix)
	if err != nil {
		return nil, err
	}

	list, err := be.List(t)
	if err != nil {
		return nil, err
	}

	match := ID(nil)

	// TODO: optimize by sorting list etc.
	for _, id := range list {
		if bytes.Equal(p, id[:len(p)]) {
			if match == nil {
				match = id
			} else {
				return nil, ErrMultipleIDMatches
			}
		}
	}

	if match != nil {
		return match, nil
	}

	return nil, ErrNoIDPrefixFound
}

// FindSnapshot takes a string and tries to find a snapshot whose ID matches
// the string as closely as possible.
func FindSnapshot(be Lister, s string) (ID, error) {
	// parse ID directly
	if id, err := ParseID(s); err == nil {
		return id, nil
	}

	// find snapshot id with prefix
	id, err := Find(be, Snapshot, s)
	if err != nil {
		return nil, err
	}

	return id, nil
}

// PrefixLength returns the number of bytes required so that all prefixes of
// all IDs of type t are unique.
func PrefixLength(be Lister, t Type) (int, error) {
	// load all IDs of the given type
	list, err := be.List(t)
	if err != nil {
		return 0, err
	}

	sort.Sort(list)

	// select prefixes of length l, test if the last one is the same as the current one
outer:
	for l := MinPrefixLength; l < IDSize; l++ {
		var last ID

		for _, id := range list {
			if bytes.Equal(last, id[:l]) {
				continue outer
			}
			last = id[:l]
		}

		return l, nil
	}

	return IDSize, nil
}
