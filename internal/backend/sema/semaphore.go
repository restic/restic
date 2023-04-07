// Package sema implements semaphores.
package sema

import (
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
)

// A semaphore limits access to a restricted resource.
type semaphore struct {
	ch chan struct{}
}

// newSemaphore returns a new semaphore with capacity n.
func newSemaphore(n uint) (semaphore, error) {
	if n == 0 {
		return semaphore{}, errors.New("capacity must be a positive number")
	}
	return semaphore{
		ch: make(chan struct{}, n),
	}, nil
}

// GetToken blocks until a Token is available.
func (s semaphore) GetToken() {
	s.ch <- struct{}{}
	debug.Log("acquired token")
}

// ReleaseToken returns a token.
func (s semaphore) ReleaseToken() { <-s.ch }
