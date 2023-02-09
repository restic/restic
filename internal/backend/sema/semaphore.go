// Package sema implements semaphores.
package sema

import (
	"context"
	"io"

	"github.com/restic/restic/internal/errors"
)

// A Semaphore limits access to a restricted resource.
type Semaphore struct {
	ch chan struct{}
}

// New returns a new semaphore with capacity n.
func New(n uint) (Semaphore, error) {
	if n == 0 {
		return Semaphore{}, errors.New("capacity must be a positive number")
	}
	return Semaphore{
		ch: make(chan struct{}, n),
	}, nil
}

// GetToken blocks until a Token is available.
func (s Semaphore) GetToken() { s.ch <- struct{}{} }

// ReleaseToken returns a token.
func (s Semaphore) ReleaseToken() { <-s.ch }

// ReleaseTokenOnClose wraps an io.ReadCloser to return a token on Close.
// Before returning the token, cancel, if not nil, will be run
// to free up context resources.
func (s Semaphore) ReleaseTokenOnClose(rc io.ReadCloser, cancel context.CancelFunc) io.ReadCloser {
	return &wrapReader{ReadCloser: rc, sem: s, cancel: cancel}
}

type wrapReader struct {
	io.ReadCloser
	eofSeen bool
	sem     Semaphore
	cancel  context.CancelFunc
}

func (wr *wrapReader) Read(p []byte) (int, error) {
	if wr.eofSeen { // XXX Why do we do this?
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
	if wr.cancel != nil {
		wr.cancel()
	}
	wr.sem.ReleaseToken()
	return err
}
