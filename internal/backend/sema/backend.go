package sema

import (
	"context"
	"io"

	"github.com/cenkalti/backoff/v4"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
)

// make sure that connectionLimitedBackend implements restic.Backend
var _ restic.Backend = &connectionLimitedBackend{}

// connectionLimitedBackend limits the number of concurrent operations.
type connectionLimitedBackend struct {
	restic.Backend
	sem semaphore
}

// NewBackend creates a backend that limits the concurrent operations on the underlying backend
func NewBackend(be restic.Backend) restic.Backend {
	sem, err := newSemaphore(be.Connections())
	if err != nil {
		panic(err)
	}

	return &connectionLimitedBackend{
		Backend: be,
		sem:     sem,
	}
}

// Save adds new Data to the backend.
func (be *connectionLimitedBackend) Save(ctx context.Context, h restic.Handle, rd restic.RewindReader) error {
	if err := h.Valid(); err != nil {
		return backoff.Permanent(err)
	}

	be.sem.GetToken()
	defer be.sem.ReleaseToken()

	return be.Backend.Save(ctx, h, rd)
}

// Load runs fn with a reader that yields the contents of the file at h at the
// given offset.
func (be *connectionLimitedBackend) Load(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
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
func (be *connectionLimitedBackend) Stat(ctx context.Context, h restic.Handle) (restic.FileInfo, error) {
	if err := h.Valid(); err != nil {
		return restic.FileInfo{}, backoff.Permanent(err)
	}

	be.sem.GetToken()
	defer be.sem.ReleaseToken()

	return be.Backend.Stat(ctx, h)
}

// Remove deletes a file from the backend.
func (be *connectionLimitedBackend) Remove(ctx context.Context, h restic.Handle) error {
	if err := h.Valid(); err != nil {
		return backoff.Permanent(err)
	}

	be.sem.GetToken()
	defer be.sem.ReleaseToken()

	return be.Backend.Remove(ctx, h)
}

func (be *connectionLimitedBackend) Unwrap() restic.Backend {
	return be.Backend
}
