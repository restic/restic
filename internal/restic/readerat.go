package restic

import (
	"context"
	"io"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
)

type backendReaderAt struct {
	be Backend
	h  Handle
}

func (brd backendReaderAt) ReadAt(p []byte, offset int64) (n int, err error) {
	return ReadAt(context.TODO(), brd.be, brd.h, offset, p)
}

// ReaderAt returns an io.ReaderAt for a file in the backend.
func ReaderAt(be Backend, h Handle) io.ReaderAt {
	return backendReaderAt{be: be, h: h}
}

// ReadAt reads from the backend handle h at the given position.
func ReadAt(ctx context.Context, be Backend, h Handle, offset int64, p []byte) (n int, err error) {
	debug.Log("ReadAt(%v) at %v, len %v", h, offset, len(p))
	rd, err := be.Load(ctx, h, len(p), offset)
	if err != nil {
		return 0, err
	}

	n, err = io.ReadFull(rd, p)
	e := rd.Close()
	if err == nil {
		err = e
	}

	debug.Log("ReadAt(%v) ReadFull returned %v bytes", h, n)

	return n, errors.Wrapf(err, "ReadFull(%v)", h)
}
