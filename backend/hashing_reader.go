package backend

import (
	"hash"
	"io"
)

type HashReader struct {
	r      io.Reader
	h      hash.Hash
	sum    []byte
	closed bool
}

func NewHashReader(r io.Reader, h hash.Hash) *HashReader {
	return &HashReader{
		h:   h,
		r:   io.TeeReader(r, h),
		sum: make([]byte, 0, h.Size()),
	}
}

func (h *HashReader) Read(p []byte) (n int, err error) {
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
