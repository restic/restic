package backend

import (
	"errors"
	"hash"
	"io"
)

type HashAppendWriter struct {
	w      io.Writer
	origWr io.Writer
	h      hash.Hash
	sum    []byte
	closed bool
}

func NewHashAppendWriter(w io.Writer, h hash.Hash) *HashAppendWriter {
	return &HashAppendWriter{
		h:      h,
		w:      io.MultiWriter(w, h),
		origWr: w,
		sum:    make([]byte, 0, h.Size()),
	}
}

func (h *HashAppendWriter) Close() error {
	if h == nil {
		return nil
	}

	if !h.closed {
		h.closed = true

		_, err := h.origWr.Write(h.h.Sum(nil))
		return err
	}

	return nil
}

func (h *HashAppendWriter) Write(p []byte) (n int, err error) {
	if !h.closed {
		return h.w.Write(p)
	}

	return 0, errors.New("Write() called on closed HashAppendWriter")
}

// HashingWriter wraps an io.Writer to hashes all data that is written to it.
type HashingWriter struct {
	w    io.Writer
	h    hash.Hash
	size int
}

// NewHashAppendWriter wraps the writer w and feeds all data written to the hash h.
func NewHashingWriter(w io.Writer, h hash.Hash) *HashingWriter {
	return &HashingWriter{
		h: h,
		w: io.MultiWriter(w, h),
	}
}

// Write wraps the write method of the underlying writer and also hashes all data.
func (h *HashingWriter) Write(p []byte) (int, error) {
	n, err := h.w.Write(p)
	h.size += n
	return n, err
}

// Sum returns the hash of all data written so far.
func (h *HashingWriter) Sum(d []byte) []byte {
	return h.h.Sum(d)
}

// Size returns the number of bytes written to the underlying writer.
func (h *HashingWriter) Size() int {
	return h.size
}
