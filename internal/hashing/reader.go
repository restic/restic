package hashing

import (
	"hash"
	"io"
)

// ReadSumer hashes all data read from the underlying reader.
type ReadSumer interface {
	io.Reader
	// Sum returns the hash of the data read so far.
	Sum(d []byte) []byte
}

type reader struct {
	io.Reader
	h hash.Hash
}

type readWriterTo struct {
	reader
	writerTo io.WriterTo
}

// NewReader returns a new ReadSummer that uses the hash h. If the underlying
// reader supports WriteTo then the returned reader will do so too.
func NewReader(r io.Reader, h hash.Hash) ReadSumer {
	rs := reader{
		Reader: io.TeeReader(r, h),
		h:      h,
	}

	if _, ok := r.(io.WriterTo); ok {
		return &readWriterTo{
			reader:   rs,
			writerTo: r.(io.WriterTo),
		}
	}

	return &rs
}

// Sum returns the hash of the data read so far.
func (h *reader) Sum(d []byte) []byte {
	return h.h.Sum(d)
}

// WriteTo reads all data into the passed writer
func (h *readWriterTo) WriteTo(w io.Writer) (int64, error) {
	return h.writerTo.WriteTo(NewWriter(w, h.h))
}
