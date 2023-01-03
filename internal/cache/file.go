package cache

import (
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"
	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
)

func (c *Cache) filename(h restic.Handle) string {
	if len(h.Name) < 2 {
		panic("Name is empty or too short")
	}
	subdir := h.Name[:2]
	return filepath.Join(c.path, cacheLayoutPaths[h.Type], subdir, h.Name)
}

func (c *Cache) canBeCached(t restic.FileType) bool {
	if c == nil {
		return false
	}

	_, ok := cacheLayoutPaths[t]
	return ok
}

// Load returns a reader that yields the contents of the file with the
// given handle. rd must be closed after use. If an error is returned, the
// ReadCloser is nil.
func (c *Cache) load(h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
	debug.Log("Load(%v, %v, %v) from cache", h, length, offset)
	if !c.canBeCached(h.Type) {
		return nil, errors.New("cannot be cached")
	}

	f, err := fs.Open(c.filename(h))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, errors.WithStack(err)
	}

	size := fi.Size()
	if size <= int64(crypto.CiphertextLength(0)) {
		_ = f.Close()
		_ = c.remove(h)
		return nil, errors.Errorf("cached file %v is truncated, removing", h)
	}

	if size < offset+int64(length) {
		_ = f.Close()
		_ = c.remove(h)
		return nil, errors.Errorf("cached file %v is too small, removing", h)
	}

	if offset > 0 {
		if _, err = f.Seek(offset, io.SeekStart); err != nil {
			_ = f.Close()
			return nil, err
		}
	}

	if length <= 0 {
		return f, nil
	}
	return backend.LimitReadCloser(f, int64(length)), nil
}

// Save saves a file in the cache.
func (c *Cache) Save(h restic.Handle, rd io.Reader) error {
	debug.Log("Save to cache: %v", h)
	if rd == nil {
		return errors.New("Save() called with nil reader")
	}
	if !c.canBeCached(h.Type) {
		return errors.New("cannot be cached")
	}

	finalname := c.filename(h)
	dir := filepath.Dir(finalname)
	err := fs.Mkdir(dir, 0700)
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
		_ = fs.Remove(f.Name())
		return errors.Wrap(err, "Copy")
	}

	if n <= int64(crypto.CiphertextLength(0)) {
		_ = f.Close()
		_ = fs.Remove(f.Name())
		debug.Log("trying to cache truncated file %v, removing", h)
		return nil
	}

	// Close, then rename. Windows doesn't like the reverse order.
	if err = f.Close(); err != nil {
		_ = fs.Remove(f.Name())
		return errors.WithStack(err)
	}

	err = fs.Rename(f.Name(), finalname)
	if err != nil {
		_ = fs.Remove(f.Name())
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

// Remove deletes a file. When the file is not cache, no error is returned.
func (c *Cache) remove(h restic.Handle) error {
	if !c.Has(h) {
		return nil
	}

	return fs.Remove(c.filename(h))
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

		if err = fs.Remove(c.filename(restic.Handle{Type: t, Name: id.String()})); err != nil {
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
func (c *Cache) Has(h restic.Handle) bool {
	if !c.canBeCached(h.Type) {
		return false
	}

	_, err := fs.Stat(c.filename(h))
	return err == nil
}
