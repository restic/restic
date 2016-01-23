package backend

import "io"

// ReadCloser wraps a reader and adds a noop Close method if rd does not implement io.Closer.
func ReadCloser(rd io.Reader) io.ReadCloser {
	return readCloser{rd}
}

// readCloser wraps a reader and adds a noop Close method if rd does not implement io.Closer.
type readCloser struct {
	io.Reader
}

func (rd readCloser) Close() error {
	if r, ok := rd.Reader.(io.Closer); ok {
		return r.Close()
	}

	return nil
}
