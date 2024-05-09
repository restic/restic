package mock

import (
	"context"
	"hash"
	"io"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/errors"
)

// Backend implements a mock backend.
type Backend struct {
	CloseFn            func() error
	IsNotExistFn       func(err error) bool
	IsPermanentErrorFn func(err error) bool
	SaveFn             func(ctx context.Context, h backend.Handle, rd backend.RewindReader) error
	OpenReaderFn       func(ctx context.Context, h backend.Handle, length int, offset int64) (io.ReadCloser, error)
	StatFn             func(ctx context.Context, h backend.Handle) (backend.FileInfo, error)
	ListFn             func(ctx context.Context, t backend.FileType, fn func(backend.FileInfo) error) error
	RemoveFn           func(ctx context.Context, h backend.Handle) error
	DeleteFn           func(ctx context.Context) error
	ConnectionsFn      func() uint
	HasherFn           func() hash.Hash
	HasAtomicReplaceFn func() bool
}

// NewBackend returns new mock Backend instance
func NewBackend() *Backend {
	be := &Backend{}
	return be
}

// Close the backend.
func (m *Backend) Close() error {
	if m.CloseFn == nil {
		return nil
	}

	return m.CloseFn()
}

func (m *Backend) Connections() uint {
	if m.ConnectionsFn == nil {
		return 2
	}

	return m.ConnectionsFn()
}

// Hasher may return a hash function for calculating a content hash for the backend
func (m *Backend) Hasher() hash.Hash {
	if m.HasherFn == nil {
		return nil
	}

	return m.HasherFn()
}

// HasAtomicReplace returns whether Save() can atomically replace files
func (m *Backend) HasAtomicReplace() bool {
	if m.HasAtomicReplaceFn == nil {
		return false
	}
	return m.HasAtomicReplaceFn()
}

// IsNotExist returns true if the error is caused by a missing file.
func (m *Backend) IsNotExist(err error) bool {
	if m.IsNotExistFn == nil {
		return false
	}

	return m.IsNotExistFn(err)
}

func (m *Backend) IsPermanentError(err error) bool {
	if m.IsPermanentErrorFn == nil {
		return false
	}

	return m.IsPermanentErrorFn(err)
}

// Save data in the backend.
func (m *Backend) Save(ctx context.Context, h backend.Handle, rd backend.RewindReader) error {
	if m.SaveFn == nil {
		return errors.New("not implemented")
	}

	return m.SaveFn(ctx, h, rd)
}

// Load runs fn with a reader that yields the contents of the file at h at the
// given offset.
func (m *Backend) Load(ctx context.Context, h backend.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	rd, err := m.openReader(ctx, h, length, offset)
	if err != nil {
		return err
	}
	err = fn(rd)
	if err != nil {
		_ = rd.Close() // ignore secondary errors closing the reader
		return err
	}
	return rd.Close()
}

func (m *Backend) openReader(ctx context.Context, h backend.Handle, length int, offset int64) (io.ReadCloser, error) {
	if m.OpenReaderFn == nil {
		return nil, errors.New("not implemented")
	}

	return m.OpenReaderFn(ctx, h, length, offset)
}

// Stat an object in the backend.
func (m *Backend) Stat(ctx context.Context, h backend.Handle) (backend.FileInfo, error) {
	if m.StatFn == nil {
		return backend.FileInfo{}, errors.New("not implemented")
	}

	return m.StatFn(ctx, h)
}

// List items of type t.
func (m *Backend) List(ctx context.Context, t backend.FileType, fn func(backend.FileInfo) error) error {
	if m.ListFn == nil {
		return nil
	}

	return m.ListFn(ctx, t, fn)
}

// Remove data from the backend.
func (m *Backend) Remove(ctx context.Context, h backend.Handle) error {
	if m.RemoveFn == nil {
		return errors.New("not implemented")
	}

	return m.RemoveFn(ctx, h)
}

// Delete all data.
func (m *Backend) Delete(ctx context.Context) error {
	if m.DeleteFn == nil {
		return errors.New("not implemented")
	}

	return m.DeleteFn(ctx)
}

// Make sure that Backend implements the backend interface.
var _ backend.Backend = &Backend{}
