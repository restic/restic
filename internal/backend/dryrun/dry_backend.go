package dryrun

import (
	"context"
	"io"
	"io/ioutil"
	"sync"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"

	"github.com/restic/restic/internal/debug"
)

type sizeMap map[restic.Handle]int

var errNotFound = errors.New("not found")

// Backend passes reads through to an underlying layer and only records
// metadata about writes. This is used for `backup --dry-run`.
// It is directly derivted from the mem backend.
type Backend struct {
	be   restic.Backend
	data sizeMap
	m    sync.Mutex
}

// New returns a new backend that saves all data in a map in memory.
func New(be restic.Backend) *Backend {
	b := &Backend{
		be:   be,
		data: make(sizeMap),
	}

	debug.Log("created new dry backend")

	return b
}

// Test returns whether a file exists.
func (be *Backend) Test(ctx context.Context, h restic.Handle) (bool, error) {
	be.m.Lock()
	defer be.m.Unlock()

	debug.Log("Test %v", h)

	if _, ok := be.data[h]; ok {
		return true, nil
	}

	return be.be.Test(ctx, h)
}

// IsNotExist returns true if the file does not exist.
func (be *Backend) IsNotExist(err error) bool {
	return errors.Cause(err) == errNotFound || be.be.IsNotExist(err)
}

// Save adds new Data to the backend.
func (be *Backend) Save(ctx context.Context, h restic.Handle, rd restic.RewindReader) error {
	if err := h.Valid(); err != nil {
		return err
	}

	be.m.Lock()
	defer be.m.Unlock()

	if h.Type == restic.ConfigFile {
		h.Name = ""
	}

	if _, ok := be.data[h]; ok {
		return errors.New("file already exists")
	}

	buf, err := ioutil.ReadAll(rd)
	if err != nil {
		return err
	}

	be.data[h] = len(buf)
	debug.Log("faked saving %v bytes at %v", len(buf), h)

	return nil
}

// Load runs fn with a reader that yields the contents of the file at h at the
// given offset.
func (be *Backend) Load(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	be.m.Lock()
	defer be.m.Unlock()

	if _, ok := be.data[h]; ok {
		return errors.New("can't read file saved on dry backend")
	}
	return be.be.Load(ctx, h, length, offset, fn)
}

// Stat returns information about a file in the backend.
func (be *Backend) Stat(ctx context.Context, h restic.Handle) (restic.FileInfo, error) {
	if err := h.Valid(); err != nil {
		return restic.FileInfo{}, err
	}

	be.m.Lock()
	defer be.m.Unlock()

	if h.Type == restic.ConfigFile {
		h.Name = ""
	}

	debug.Log("stat %v", h)

	s, ok := be.data[h]
	if !ok {
		return be.be.Stat(ctx, h)
	}

	return restic.FileInfo{Size: int64(s), Name: h.Name}, nil
}

// Remove deletes a file from the backend.
func (be *Backend) Remove(ctx context.Context, h restic.Handle) error {
	be.m.Lock()
	defer be.m.Unlock()

	debug.Log("Remove %v", h)

	if _, ok := be.data[h]; !ok {
		return errNotFound
	}

	delete(be.data, h)

	return nil
}

// List returns a channel which yields entries from the backend.
func (be *Backend) List(ctx context.Context, t restic.FileType, fn func(restic.FileInfo) error) error {
	entries := []restic.FileInfo{}
	be.m.Lock()
	for entry, size := range be.data {
		if entry.Type != t {
			continue
		}
		entries = append(entries, restic.FileInfo{
			Name: entry.Name,
			Size: int64(size),
		})
	}
	be.m.Unlock()

	for _, entry := range entries {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		err := fn(entry)
		if err != nil {
			return err
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	return be.be.List(ctx, t, fn)
}

// Location returns the location of the backend (RAM).
func (be *Backend) Location() string {
	return "DRY:" + be.be.Location()
}

// Delete removes all data in the backend.
func (be *Backend) Delete(ctx context.Context) error {
	return errors.New("dry-run doesn't support Delete()")
}

// Close closes the backend.
func (be *Backend) Close() error {
	return be.be.Close()
}
