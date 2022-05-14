package hashing

import (
	"hash"
	"io"
)

// A Reader hashes all data read from the underlying reader.
type Reader struct {
	r io.Reader
	h hash.Hash
}

// NewReader returns a new Reader that uses the hash h.
func NewReader(r io.Reader, h hash.Hash) *Reader {
	return &Reader{r: r, h: h}
}

func (h *Reader) Read(p []byte) (int, error) {
	n, err := h.r.Read(p)
	_, _ = h.h.Write(p[:n]) // Never returns an error.
	return n, err
}

// Sum returns the hash of the data read so far.
func (h *Reader) Sum(d []byte) []byte {
	return h.h.Sum(d)
}
