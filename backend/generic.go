package backend

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/ioutil"
	"sync"
)

var idPool = sync.Pool{New: func() interface{} { return ID(make([]byte, IDSize)) }}

var (
	ErrNoIDPrefixFound   = errors.New("no ID found")
	ErrMultipleIDMatches = errors.New("multiple IDs with prefix found")
)

// Each lists all entries of type t in the backend and calls function f() with
// the id and data.
func Each(be Server, t Type, f func(id ID, data []byte, err error)) error {
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
func EachID(be Server, t Type, f func(ID)) error {
	ids, err := be.List(t)
	if err != nil {
		return err
	}

	for _, id := range ids {
		f(id)
	}

	return nil
}

// Compress applies zlib compression to data.
func Compress(data []byte) []byte {
	// apply zlib compression
	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	_, err := w.Write(data)
	if err != nil {
		panic(err)
	}
	w.Close()

	return b.Bytes()
}

// Uncompress reverses zlib compression on data.
func Uncompress(data []byte) []byte {
	b := bytes.NewBuffer(data)
	r, err := zlib.NewReader(b)
	if err != nil {
		panic(err)
	}

	buf, err := ioutil.ReadAll(r)
	if err != nil {
		panic(err)
	}

	r.Close()

	return buf
}

// Hash returns the ID for data.
func Hash(data []byte) ID {
	h := sha256.Sum256(data)
	id := idPool.Get().(ID)
	copy(id, h[:])
	return id
}

// Find loads the list of all blobs of type t and searches for IDs which start
// with prefix. If none is found, nil and ErrNoIDPrefixFound is returned. If
// more than one is found, nil and ErrMultipleIDMatches is returned.
func Find(be Server, t Type, prefix string) (ID, error) {
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
func FindSnapshot(be Server, s string) (ID, error) {
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
