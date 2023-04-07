package sema

import (
	"context"
	"io"

	"github.com/cenkalti/backoff/v4"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
)

// make sure that SemaphoreBackend implements restic.Backend
var _ restic.Backend = &SemaphoreBackend{}

// SemaphoreBackend limits the number of concurrent operations.
type SemaphoreBackend struct {
	restic.Backend
	sem semaphore
}

// New creates a backend that limits the concurrent operations on the underlying backend
func New(be restic.Backend) *SemaphoreBackend {
	sem, err := newSemaphore(be.Connections())
	if err != nil {
		panic(err)
	}

	return &SemaphoreBackend{
		Backend: be,
		sem:     sem,
	}
}

// Save adds new Data to the backend.
func (be *SemaphoreBackend) Save(ctx context.Context, h restic.Handle, rd restic.RewindReader) error {
	if err := h.Valid(); err != nil {
		return backoff.Permanent(err)
	}

	be.sem.GetToken()
	defer be.sem.ReleaseToken()

	return be.Backend.Save(ctx, h, rd)
}

// Load runs fn with a reader that yields the contents of the file at h at the
// given offset.
func (be *SemaphoreBackend) Load(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	if err := h.Valid(); err != nil {
		return backoff.Permanent(err)
	}
	if offset < 0 {
		return backoff.Permanent(errors.New("offset is negative"))
	}
	if length < 0 {
		return backoff.Permanent(errors.Errorf("invalid length %d", length))
	}

	be.sem.GetToken()
	defer be.sem.ReleaseToken()

	return be.Backend.Load(ctx, h, length, offset, fn)
}

// Stat returns information about a file in the backend.
func (be *SemaphoreBackend) Stat(ctx context.Context, h restic.Handle) (restic.FileInfo, error) {
	if err := h.Valid(); err != nil {
		return restic.FileInfo{}, backoff.Permanent(err)
	}

	be.sem.GetToken()
	defer be.sem.ReleaseToken()

	return be.Backend.Stat(ctx, h)
}

// Remove deletes a file from the backend.
func (be *SemaphoreBackend) Remove(ctx context.Context, h restic.Handle) error {
	if err := h.Valid(); err != nil {
		return backoff.Permanent(err)
	}

	be.sem.GetToken()
	defer be.sem.ReleaseToken()

	return be.Backend.Remove(ctx, h)
}
