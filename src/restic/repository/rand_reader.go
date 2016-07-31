package repository

import (
	"io"
	"math/rand"
)

// RandReader allows reading from a rand.Rand.
type RandReader struct {
	rnd *rand.Rand
	buf []byte
}

// NewRandReader creates a new Reader from a random source.
func NewRandReader(rnd *rand.Rand) io.Reader {
	return &RandReader{rnd: rnd, buf: make([]byte, 0, 7)}
}

func (rd *RandReader) read(p []byte) (n int, err error) {
	if len(p)%7 != 0 {
		panic("invalid buffer length, not multiple of 7")
	}

	rnd := rd.rnd
	for i := 0; i < len(p); i += 7 {
		val := rnd.Int63()

		p[i+0] = byte(val >> 0)
		p[i+1] = byte(val >> 8)
		p[i+2] = byte(val >> 16)
		p[i+3] = byte(val >> 24)
		p[i+4] = byte(val >> 32)
		p[i+5] = byte(val >> 40)
		p[i+6] = byte(val >> 48)
	}

	return len(p), nil
}

func (rd *RandReader) Read(p []byte) (int, error) {
	// first, copy buffer to p
	pos := copy(p, rd.buf)
	copy(rd.buf, rd.buf[pos:])

	// shorten buf and p accordingly
	rd.buf = rd.buf[:len(rd.buf)-pos]
	p = p[pos:]

	// if this is enough to fill p, return
	if len(p) == 0 {
		return pos, nil
	}

	// load multiple of 7 byte
	l := (len(p) / 7) * 7
	n, err := rd.read(p[:l])
	pos += n
	if err != nil {
		return pos, err
	}
	p = p[n:]

	// load 7 byte to temp buffer
	rd.buf = rd.buf[:7]
	n, err = rd.read(rd.buf)
	if err != nil {
		return pos, err
	}

	// copy the remaining bytes from the buffer to p
	n = copy(p, rd.buf)
	pos += n

	// save the remaining bytes in rd.buf
	n = copy(rd.buf, rd.buf[n:])
	rd.buf = rd.buf[:n]

	return pos, nil
}
