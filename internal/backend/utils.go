package backend

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"

	"github.com/restic/restic/internal/restic"
)

// LoadAll reads all data stored in the backend for the handle into the given
// buffer, which is truncated. If the buffer is not large enough or nil, a new
// one is allocated.
func LoadAll(ctx context.Context, buf []byte, be restic.Backend, h restic.Handle) ([]byte, error) {
	rd, err := be.Load(ctx, h, 0, 0)
	if err != nil {
		return nil, err
	}

	defer func() {
		_, e := io.Copy(ioutil.Discard, rd)
		if err == nil {
			err = e
		}

		e = rd.Close()
		if err == nil {
			err = e
		}
	}()

	bbuf := bytes.NewBuffer(buf[:0])
	_, err = io.Copy(bbuf, rd)
	return bbuf.Bytes(), err
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
