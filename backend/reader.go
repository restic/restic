package backend

import (
	"hash"
	"io"
)

type HashAppendReader struct {
	r      io.Reader
	h      hash.Hash
	sum    []byte
	closed bool
}

func NewHashAppendReader(r io.Reader, h hash.Hash) *HashAppendReader {
	return &HashAppendReader{
		h:   h,
		r:   io.TeeReader(r, h),
		sum: make([]byte, 0, h.Size()),
	}
}

func (h *HashAppendReader) Read(p []byte) (n int, err error) {
	if !h.closed {
		n, err = h.r.Read(p)

		if err == io.EOF {
			h.closed = true
			h.sum = h.h.Sum(h.sum)
		} else if err != nil {
			return
		}
	}

	if h.closed {
		// output hash
		r := len(p) - n

		if r > 0 {
			c := copy(p[n:], h.sum)
			h.sum = h.sum[c:]

			n += c
			err = nil
		}

		if len(h.sum) == 0 {
			err = io.EOF
		}
	}

	return
}

type HashingReader struct {
	r io.Reader
	h hash.Hash
}

func NewHashingReader(r io.Reader, h hash.Hash) *HashingReader {
	return &HashingReader{
		h: h,
		r: io.TeeReader(r, h),
	}
}

func (h *HashingReader) Read(p []byte) (int, error) {
	return h.r.Read(p)
}

func (h *HashingReader) Sum(d []byte) []byte {
	return h.h.Sum(d)
}
