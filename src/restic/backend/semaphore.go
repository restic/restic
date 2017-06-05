package backend

import "restic/errors"

// Semaphore limits access to a restricted resource.
type Semaphore struct {
	ch chan struct{}
}

// NewSemaphore returns a new semaphore with capacity n.
func NewSemaphore(n uint) (*Semaphore, error) {
	if n <= 0 {
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
