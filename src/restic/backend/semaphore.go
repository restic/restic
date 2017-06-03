package backend

// Semaphore limits access to a restricted resource.
type Semaphore struct {
	ch chan struct{}
}

// NewSemaphore returns a new semaphore with capacity n.
func NewSemaphore(n int) *Semaphore {
	return &Semaphore{
		ch: make(chan struct{}, n),
	}
}

// GetToken blocks until a Token is available.
func (s *Semaphore) GetToken() {
	s.ch <- struct{}{}
}

// ReleaseToken returns a token.
func (s *Semaphore) ReleaseToken() {
	<-s.ch
}
