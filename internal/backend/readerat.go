package backend

import (
	"context"
	"io"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
)

type backendReaderAt struct {
	ctx context.Context
	be  Backend
	h   Handle
}

func (brd backendReaderAt) ReadAt(p []byte, offset int64) (n int, err error) {
	return ReadAt(brd.ctx, brd.be, brd.h, offset, p)
}

// ReaderAt returns an io.ReaderAt for a file in the backend. The returned reader
// should not escape the caller function to avoid unexpected interactions with the
// embedded context
func ReaderAt(ctx context.Context, be Backend, h Handle) io.ReaderAt {
	return backendReaderAt{ctx: ctx, be: be, h: h}
}

// ReadAt reads from the backend handle h at the given position.
func ReadAt(ctx context.Context, be Backend, h Handle, offset int64, p []byte) (n int, err error) {
	debug.Log("ReadAt(%v) at %v, len %v", h, offset, len(p))

	err = be.Load(ctx, h, len(p), offset, func(rd io.Reader) (ierr error) {
		n, ierr = io.ReadFull(rd, p)

		return ierr
	})
	if err != nil {
		return 0, errors.Wrapf(err, "ReadFull(%v)", h)
	}

	debug.Log("ReadAt(%v) ReadFull returned %v bytes", h, n)
	return n, nil
}
