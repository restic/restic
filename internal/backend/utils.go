package backend

import (
	"context"
	"io"
	"io/ioutil"

	"github.com/restic/restic/internal/restic"
)

// LoadAll reads all data stored in the backend for the handle.
func LoadAll(ctx context.Context, be restic.Backend, h restic.Handle) (buf []byte, err error) {
	err = be.Load(ctx, h, 0, 0, func(rd io.Reader) (ierr error) {
		buf, ierr = ioutil.ReadAll(rd)
		return ierr
	})
	return buf, err
}

// LimitedReadCloser wraps io.LimitedReader and exposes the Close() method.
type LimitedReadCloser struct {
	io.ReadCloser
	io.Reader
}

// Read reads data from the limited reader.
func (l *LimitedReadCloser) Read(p []byte) (int, error) {
	return l.Reader.Read(p)
}

// LimitReadCloser returns a new reader wraps r in an io.LimitReader, but also
// exposes the Close() method.
func LimitReadCloser(r io.ReadCloser, n int64) *LimitedReadCloser {
	return &LimitedReadCloser{ReadCloser: r, Reader: io.LimitReader(r, n)}
}

// DefaultLoad implements Backend.Load using lower-level openReader func
func DefaultLoad(ctx context.Context, h restic.Handle, length int, offset int64,
	openReader func(ctx context.Context, h restic.Handle, length int, offset int64) (io.ReadCloser, error),
	fn func(rd io.Reader) error) error {
	rd, err := openReader(ctx, h, length, offset)
	if err != nil {
		return err
	}
	err = fn(rd)
	if err != nil {
		rd.Close() // ignore secondary errors closing the reader
		return err
	}
	return rd.Close()
}
