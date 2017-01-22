package backend

import (
	"io"
	"restic"

	"restic/errors"
)

// LoadAll reads all data stored in the backend for the handle. The buffer buf
// is resized to accomodate all data in the blob. Errors returned by be.Load()
// are passed on, except io.ErrUnexpectedEOF is silenced and nil returned
// instead, since it means this function is working properly.
func LoadAll(be restic.Backend, h restic.Handle, buf []byte) ([]byte, error) {
	fi, err := be.Stat(h)
	if err != nil {
		return nil, errors.Wrap(err, "Stat")
	}

	if fi.Size > int64(len(buf)) {
		buf = make([]byte, int(fi.Size))
	}

	n, err := be.Load(h, buf, 0)
	if errors.Cause(err) == io.ErrUnexpectedEOF {
		err = nil
	}
	buf = buf[:n]
	return buf, err
}

// Closer wraps an io.Reader and adds a Close() method that does nothing.
type Closer struct {
	io.Reader
}

// Close is a no-op.
func (c Closer) Close() error {
	return nil
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
