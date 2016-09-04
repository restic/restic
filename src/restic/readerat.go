package restic

import (
	"io"
)

type backendReaderAt struct {
	be Backend
	h  Handle
}

func (brd backendReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	return brd.be.Load(brd.h, p, off)
}

// ReaderAt returns an io.ReaderAt for a file in the backend.
func ReaderAt(be Backend, h Handle) io.ReaderAt {
	return backendReaderAt{be: be, h: h}
}
