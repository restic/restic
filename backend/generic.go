package backend

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"io/ioutil"
	"sync"
)

var idPool = sync.Pool{New: func() interface{} { return ID(make([]byte, IDSize)) }}

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
