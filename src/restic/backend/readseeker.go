package backend

import (
	"errors"
	"io"
)

type readSeeker struct {
	be     Backend
	h      Handle
	t      Type
	name   string
	offset int64
	size   int64
}

// NewReadSeeker returns an io.ReadSeeker for the given object in the backend.
func NewReadSeeker(be Backend, h Handle) io.ReadSeeker {
	return &readSeeker{be: be, h: h}
}

func (rd *readSeeker) Read(p []byte) (int, error) {
	n, err := rd.be.Load(rd.h, p, rd.offset)
	rd.offset += int64(n)
	return n, err
}

func (rd *readSeeker) Seek(offset int64, whence int) (n int64, err error) {
	switch whence {
	case 0:
		rd.offset = offset
	case 1:
		rd.offset += offset
	case 2:
		if rd.size == 0 {
			rd.size, err = rd.getSize()
			if err != nil {
				return 0, err
			}
		}

		pos := rd.size + offset
		if pos < 0 {
			return 0, errors.New("invalid offset, before start of blob")
		}

		rd.offset = pos
		return rd.offset, nil
	default:
		return 0, errors.New("invalid value for parameter whence")
	}

	return rd.offset, nil
}

func (rd *readSeeker) getSize() (int64, error) {
	stat, err := rd.be.Stat(rd.h)
	if err != nil {
		return 0, err
	}

	return stat.Size, nil
}
