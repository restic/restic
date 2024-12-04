package cache

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"
	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/util"
	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
)

func (c *Cache) filename(h backend.Handle) string {
	if len(h.Name) < 2 {
		panic("Name is empty or too short")
	}
	subdir := h.Name[:2]
	return filepath.Join(c.path, cacheLayoutPaths[h.Type], subdir, h.Name)
}

func (c *Cache) canBeCached(t backend.FileType) bool {
	if c == nil {
		return false
	}

	_, ok := cacheLayoutPaths[t]
	return ok
}

// load returns a reader that yields the contents of the file with the
// given handle. rd must be closed after use. If an error is returned, the
// ReadCloser is nil. The bool return value indicates whether the requested
// file exists in the cache. It can be true even when no reader is returned
// because length or offset are out of bounds
func (c *Cache) load(h backend.Handle, length int, offset int64) (io.ReadCloser, bool, error) {
	debug.Log("Load(%v, %v, %v) from cache", h, length, offset)
	if !c.canBeCached(h.Type) {
		return nil, false, errors.New("cannot be cached")
	}

	f, err := os.Open(c.filename(h))
	if err != nil {
		return nil, false, errors.WithStack(err)
	}

	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, true, errors.WithStack(err)
	}

	size := fi.Size()
	if size <= int64(crypto.CiphertextLength(0)) {
		_ = f.Close()
		return nil, true, errors.Errorf("cached file %v is truncated", h)
	}

	if size < offset+int64(length) {
		_ = f.Close()
		return nil, true, errors.Errorf("cached file %v is too short", h)
	}

	if offset > 0 {
		if _, err = f.Seek(offset, io.SeekStart); err != nil {
			_ = f.Close()
			return nil, true, err
		}
	}

	if length <= 0 {
		return f, true, nil
	}
	return util.LimitReadCloser(f, int64(length)), true, nil
}

// save saves a file in the cache.
func (c *Cache) save(h backend.Handle, rd io.Reader) error {
	debug.Log("Save to cache: %v", h)
	if rd == nil {
		return errors.New("Save() called with nil reader")
	}
	if !c.canBeCached(h.Type) {
		return errors.New("cannot be cached")
	}

	finalname := c.filename(h)
	dir := filepath.Dir(finalname)
	err := os.Mkdir(dir, 0700)
	if err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}

	// First save to a temporary location. This allows multiple concurrent
	// restics to use a single cache dir.
	f, err := os.CreateTemp(dir, "tmp-")
	if err != nil {
		return err
	}

	n, err := io.Copy(f, rd)
	if err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return errors.Wrap(err, "Copy")
	}

	if n <= int64(crypto.CiphertextLength(0)) {
		_ = f.Close()
		_ = os.Remove(f.Name())
		debug.Log("trying to cache truncated file %v, removing", h)
		return nil
	}

	// Close, then rename. Windows doesn't like the reverse order.
	if err = f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return errors.WithStack(err)
	}

	err = os.Rename(f.Name(), finalname)
	if err != nil {
		_ = os.Remove(f.Name())
	}
	if runtime.GOOS == "windows" && errors.Is(err, os.ErrPermission) {
		// On Windows, renaming over an existing file is ok
		// (os.Rename is MoveFileExW with MOVEFILE_REPLACE_EXISTING
		// since Go 1.5), but not when someone else has the file open.
		//
		// When we get Access denied, we assume that's the case
		// and the other process has written the desired contents to f.
		err = nil
	}

	return errors.WithStack(err)
}

func (c *Cache) Forget(h backend.Handle) error {
	h.IsMetadata = false

	if _, ok := c.forgotten.Load(h); ok {
		// Delete a file at most once while restic runs.
		// This prevents repeatedly caching and forgetting broken files
		return fmt.Errorf("circuit breaker prevents repeated deletion of cached file %v", h)
	}

	removed, err := c.remove(h)
	if removed {
		c.forgotten.Store(h, struct{}{})
	}
	return err
}

// remove deletes a file. When the file is not cached, no error is returned.
func (c *Cache) remove(h backend.Handle) (bool, error) {
	if !c.canBeCached(h.Type) {
		return false, nil
	}

	err := os.Remove(c.filename(h))
	removed := err == nil
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	return removed, err
}

// Clear removes all files of type t from the cache that are not contained in
// the set valid.
func (c *Cache) Clear(t restic.FileType, valid restic.IDSet) error {
	debug.Log("Clearing cache for %v: %v valid files", t, len(valid))
	if !c.canBeCached(t) {
		return nil
	}

	list, err := c.list(t)
	if err != nil {
		return err
	}

	for id := range list {
		if valid.Has(id) {
			continue
		}

		// ignore ErrNotExist to gracefully handle multiple processes running Clear() concurrently
		if err = os.Remove(c.filename(backend.Handle{Type: t, Name: id.String()})); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	return nil
}

func isFile(fi os.FileInfo) bool {
	return fi.Mode()&(os.ModeType|os.ModeCharDevice) == 0
}

// list returns a list of all files of type T in the cache.
func (c *Cache) list(t restic.FileType) (restic.IDSet, error) {
	if !c.canBeCached(t) {
		return nil, errors.New("cannot be cached")
	}

	list := restic.NewIDSet()
	dir := filepath.Join(c.path, cacheLayoutPaths[t])
	err := filepath.Walk(dir, func(name string, fi os.FileInfo, err error) error {
		if err != nil {
			// ignore ErrNotExist to gracefully handle multiple processes clearing the cache
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return errors.Wrap(err, "Walk")
		}

		if !isFile(fi) {
			return nil
		}

		id, err := restic.ParseID(filepath.Base(name))
		if err != nil {
			return nil
		}

		list.Insert(id)
		return nil
	})

	return list, err
}

// Has returns true if the file is cached.
func (c *Cache) Has(h backend.Handle) bool {
	if !c.canBeCached(h.Type) {
		return false
	}

	_, err := os.Stat(c.filename(h))
	return err == nil
}
