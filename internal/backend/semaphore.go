package backend

import (
	"context"
	"io"

	"github.com/restic/restic/internal/errors"
)

// Semaphore limits access to a restricted resource.
type Semaphore struct {
	ch chan struct{}
}

// NewSemaphore returns a new semaphore with capacity n.
func NewSemaphore(n uint) (*Semaphore, error) {
	if n == 0 {
		return nil, errors.New("must be a positive number")
	}
	return &Semaphore{
		ch: make(chan struct{}, n),
	}, nil
}

// GetToken blocks until a Token is available.
func (s *Semaphore) GetToken() {
	s.ch <- struct{}{}
}

// ReleaseToken returns a token.
func (s *Semaphore) ReleaseToken() {
	<-s.ch
}

// ReleaseTokenOnClose wraps an io.ReadCloser to return a token on Close. Before returning the token,
// cancel, if provided, will be run to free up context resources.
func (s *Semaphore) ReleaseTokenOnClose(rc io.ReadCloser, cancel context.CancelFunc) io.ReadCloser {
	return &wrapReader{rc, false, func() {
		if cancel != nil {
			cancel()
		}
		s.ReleaseToken()
	}}
}

// wrapReader wraps an io.ReadCloser to run an additional function on Close.
type wrapReader struct {
	io.ReadCloser
	eofSeen bool
	f       func()
}

func (wr *wrapReader) Read(p []byte) (int, error) {
	if wr.eofSeen {
		return 0, io.EOF
	}

	n, err := wr.ReadCloser.Read(p)
	if err == io.EOF {
		wr.eofSeen = true
	}
	return n, err
}

func (wr *wrapReader) Close() error {
	err := wr.ReadCloser.Close()
	wr.f()
	return err
}
