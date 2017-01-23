package restic

import (
	"io"
)

type backendReaderAt struct {
	be Backend
	h  Handle
}

func (brd backendReaderAt) ReadAt(p []byte, offset int64) (n int, err error) {
	return ReadAt(brd.be, brd.h, offset, p)
}

// ReaderAt returns an io.ReaderAt for a file in the backend.
func ReaderAt(be Backend, h Handle) io.ReaderAt {
	return backendReaderAt{be: be, h: h}
}

// ReadAt reads from the backend handle h at the given position.
func ReadAt(be Backend, h Handle, offset int64, p []byte) (n int, err error) {
	rd, err := be.Get(h, len(p), offset)
	if err != nil {
		return 0, err
	}

	n, err = io.ReadFull(rd, p)
	e := rd.Close()
	if err == nil {
		err = e
	}

	return n, err
}
