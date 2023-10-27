package sema

import (
	"context"
	"io"
	"sync"

	"github.com/cenkalti/backoff/v4"
	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/errors"
)

// make sure that connectionLimitedBackend implements backend.Backend
var _ backend.Backend = &connectionLimitedBackend{}

// connectionLimitedBackend limits the number of concurrent operations.
type connectionLimitedBackend struct {
	backend.Backend
	sem        semaphore
	freezeLock sync.Mutex
}

// NewBackend creates a backend that limits the concurrent operations on the underlying backend
func NewBackend(be backend.Backend) backend.Backend {
	sem, err := newSemaphore(be.Connections())
	if err != nil {
		panic(err)
	}

	return &connectionLimitedBackend{
		Backend: be,
		sem:     sem,
	}
}

// typeDependentLimit acquire a token unless the FileType is a lock file. The returned function
// must be called to release the token.
func (be *connectionLimitedBackend) typeDependentLimit(t backend.FileType) func() {
	// allow concurrent lock file operations to ensure that the lock refresh is always possible
	if t == backend.LockFile {
		return func() {}
	}
	be.sem.GetToken()
	// prevent token usage while the backend is frozen
	be.freezeLock.Lock()
	defer be.freezeLock.Unlock()

	return be.sem.ReleaseToken
}

// Freeze blocks all backend operations except those on lock files
func (be *connectionLimitedBackend) Freeze() {
	be.freezeLock.Lock()
}

// Unfreeze allows all backend operations to continue
func (be *connectionLimitedBackend) Unfreeze() {
	be.freezeLock.Unlock()
}

// Save adds new Data to the backend.
func (be *connectionLimitedBackend) Save(ctx context.Context, h backend.Handle, rd backend.RewindReader) error {
	if err := h.Valid(); err != nil {
		return backoff.Permanent(err)
	}

	defer be.typeDependentLimit(h.Type)()

	if ctx.Err() != nil {
		return ctx.Err()
	}

	return be.Backend.Save(ctx, h, rd)
}

// Load runs fn with a reader that yields the contents of the file at h at the
// given offset.
func (be *connectionLimitedBackend) Load(ctx context.Context, h backend.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	if err := h.Valid(); err != nil {
		return backoff.Permanent(err)
	}
	if offset < 0 {
		return backoff.Permanent(errors.New("offset is negative"))
	}
	if length < 0 {
		return backoff.Permanent(errors.Errorf("invalid length %d", length))
	}

	defer be.typeDependentLimit(h.Type)()

	if ctx.Err() != nil {
		return ctx.Err()
	}

	return be.Backend.Load(ctx, h, length, offset, fn)
}

// Stat returns information about a file in the backend.
func (be *connectionLimitedBackend) Stat(ctx context.Context, h backend.Handle) (backend.FileInfo, error) {
	if err := h.Valid(); err != nil {
		return backend.FileInfo{}, backoff.Permanent(err)
	}

	defer be.typeDependentLimit(h.Type)()

	if ctx.Err() != nil {
		return backend.FileInfo{}, ctx.Err()
	}

	return be.Backend.Stat(ctx, h)
}

// Remove deletes a file from the backend.
func (be *connectionLimitedBackend) Remove(ctx context.Context, h backend.Handle) error {
	if err := h.Valid(); err != nil {
		return backoff.Permanent(err)
	}

	defer be.typeDependentLimit(h.Type)()

	if ctx.Err() != nil {
		return ctx.Err()
	}

	return be.Backend.Remove(ctx, h)
}

func (be *connectionLimitedBackend) Unwrap() backend.Backend {
	return be.Backend
}
