package dryrun

import (
	"context"
	"hash"
	"io"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
)

// Backend passes reads through to an underlying layer and accepts writes, but
// doesn't do anything. Also removes are ignored.
// So in fact, this backend silently ignores all operations that would modify
// the repo and does normal operations else.
// This is used for `backup --dry-run`.
type Backend struct {
	b restic.Backend
}

// statically ensure that RetryBackend implements restic.Backend.
var _ restic.Backend = &Backend{}

// New returns a new backend that saves all data in a map in memory.
func New(be restic.Backend) *Backend {
	b := &Backend{b: be}
	debug.Log("created new dry backend")
	return b
}

// Save adds new Data to the backend.
func (be *Backend) Save(ctx context.Context, h restic.Handle, rd restic.RewindReader) error {
	if err := h.Valid(); err != nil {
		return err
	}

	debug.Log("faked saving %v bytes at %v", rd.Length(), h)

	// don't save anything, just return ok
	return nil
}

// Remove deletes a file from the backend.
func (be *Backend) Remove(ctx context.Context, h restic.Handle) error {
	return nil
}

func (be *Backend) Connections() uint {
	return be.b.Connections()
}

// Location returns the location of the backend.
func (be *Backend) Location() string {
	return "DRY:" + be.b.Location()
}

// Delete removes all data in the backend.
func (be *Backend) Delete(ctx context.Context) error {
	return nil
}

func (be *Backend) Close() error {
	return be.b.Close()
}

func (be *Backend) Hasher() hash.Hash {
	return be.b.Hasher()
}

func (be *Backend) HasAtomicReplace() bool {
	return be.b.HasAtomicReplace()
}

func (be *Backend) IsNotExist(err error) bool {
	return be.b.IsNotExist(err)
}

func (be *Backend) List(ctx context.Context, t restic.FileType, fn func(restic.FileInfo) error) error {
	return be.b.List(ctx, t, fn)
}

func (be *Backend) Load(ctx context.Context, h restic.Handle, length int, offset int64, fn func(io.Reader) error) error {
	return be.b.Load(ctx, h, length, offset, fn)
}

func (be *Backend) Stat(ctx context.Context, h restic.Handle) (restic.FileInfo, error) {
	return be.b.Stat(ctx, h)
}
