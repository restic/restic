package mock

import (
	"context"
	"io"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
)

// Backend implements a mock backend.
type Backend struct {
	CloseFn      func() error
	IsNotExistFn func(err error) bool
	SaveFn       func(ctx context.Context, h restic.Handle, rd restic.RewindReader) error
	OpenReaderFn func(ctx context.Context, h restic.Handle, length int, offset int64) (io.ReadCloser, error)
	StatFn       func(ctx context.Context, h restic.Handle) (restic.FileInfo, error)
	ListFn       func(ctx context.Context, t restic.FileType, fn func(restic.FileInfo) error) error
	RemoveFn     func(ctx context.Context, h restic.Handle) error
	TestFn       func(ctx context.Context, h restic.Handle) (bool, error)
	DeleteFn     func(ctx context.Context) error
	LocationFn   func() string
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

// Location returns a location string.
func (m *Backend) Location() string {
	if m.LocationFn == nil {
		return ""
	}

	return m.LocationFn()
}

// IsNotExist returns true if the error is caused by a missing file.
func (m *Backend) IsNotExist(err error) bool {
	if m.IsNotExistFn == nil {
		return false
	}

	return m.IsNotExistFn(err)
}

// Save data in the backend.
func (m *Backend) Save(ctx context.Context, h restic.Handle, rd restic.RewindReader) error {
	if m.SaveFn == nil {
		return errors.New("not implemented")
	}

	return m.SaveFn(ctx, h, rd)
}

// Load runs fn with a reader that yields the contents of the file at h at the
// given offset.
func (m *Backend) Load(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	rd, err := m.openReader(ctx, h, length, offset)
	if err != nil {
		return err
	}
	err = fn(rd)
	if err != nil {
		rd.Close() // ignore secondary errors closing the reader
		return err
	}
	return rd.Close()
}

func (m *Backend) openReader(ctx context.Context, h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
	if m.OpenReaderFn == nil {
		return nil, errors.New("not implemented")
	}

	return m.OpenReaderFn(ctx, h, length, offset)
}

// Stat an object in the backend.
func (m *Backend) Stat(ctx context.Context, h restic.Handle) (restic.FileInfo, error) {
	if m.StatFn == nil {
		return restic.FileInfo{}, errors.New("not implemented")
	}

	return m.StatFn(ctx, h)
}

// List items of type t.
func (m *Backend) List(ctx context.Context, t restic.FileType, fn func(restic.FileInfo) error) error {
	if m.ListFn == nil {
		return nil
	}

	return m.ListFn(ctx, t, fn)
}

// Remove data from the backend.
func (m *Backend) Remove(ctx context.Context, h restic.Handle) error {
	if m.RemoveFn == nil {
		return errors.New("not implemented")
	}

	return m.RemoveFn(ctx, h)
}

// Test for the existence of a specific item.
func (m *Backend) Test(ctx context.Context, h restic.Handle) (bool, error) {
	if m.TestFn == nil {
		return false, errors.New("not implemented")
	}

	return m.TestFn(ctx, h)
}

// Delete all data.
func (m *Backend) Delete(ctx context.Context) error {
	if m.DeleteFn == nil {
		return errors.New("not implemented")
	}

	return m.DeleteFn(ctx)
}

// Make sure that Backend implements the backend interface.
var _ restic.Backend = &Backend{}
