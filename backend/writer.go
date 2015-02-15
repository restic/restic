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

type HashingWriter struct {
	w io.Writer
	h hash.Hash
}

func NewHashingWriter(w io.Writer, h hash.Hash) *HashingWriter {
	return &HashingWriter{
		h: h,
		w: io.MultiWriter(w, h),
	}
}

func (h *HashingWriter) Write(p []byte) (int, error) {
	return h.w.Write(p)
}

func (h *HashingWriter) Sum(d []byte) []byte {
	return h.h.Sum(d)
}
