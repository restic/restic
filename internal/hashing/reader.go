package hashing

import (
	"hash"
	"io"
)

// Reader hashes all data read from the underlying reader.
type Reader struct {
	r io.Reader
	h hash.Hash
}

// NewReader returns a new Reader that uses the hash h.
func NewReader(r io.Reader, h hash.Hash) *Reader {
	return &Reader{
		h: h,
		r: io.TeeReader(r, h),
	}
}

func (h *Reader) Read(p []byte) (int, error) {
	return h.r.Read(p)
}

// Sum returns the hash of the data read so far.
func (h *Reader) Sum(d []byte) []byte {
	return h.h.Sum(d)
}
