package hashing

import (
	"hash"
	"io"
)

// Writer transparently hashes all data while writing it to the underlying writer.
type Writer struct {
	w io.Writer
	h hash.Hash
}

// NewWriter wraps the writer w and feeds all data written to the hash h.
func NewWriter(w io.Writer, h hash.Hash) *Writer {
	return &Writer{
		h: h,
		w: w,
	}
}

// Write wraps the write method of the underlying writer and also hashes all data.
func (h *Writer) Write(p []byte) (int, error) {
	// write the data to the underlying writing
	n, err := h.w.Write(p)

	// according to the interface documentation, Write() on a hash.Hash never
	// returns an error.
	_, hashErr := h.h.Write(p[:n])
	if hashErr != nil {
		panic(hashErr)
	}

	return n, err
}

// Sum returns the hash of all data written so far.
func (h *Writer) Sum(d []byte) []byte {
	return h.h.Sum(d)
}
