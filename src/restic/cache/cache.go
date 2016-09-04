// Package cache implements a local cache for data.
package cache

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"restic"
	"restic/debug"
	"restic/errors"
	"restic/fs"
)

// Cache is a local cache implementation.
type Cache struct {
	dir string
}

// make sure that Cache implement restic.Cache
var _ restic.Cache = &Cache{}

// getXDGCacheDir returns the cache directory according to XDG basedir spec, see
// http://standards.freedesktop.org/basedir-spec/basedir-spec-latest.html
func getXDGCacheDir() (string, error) {
	xdgcache := os.Getenv("XDG_CACHE_HOME")
	home := os.Getenv("HOME")

	if xdgcache == "" && home == "" {
		return "", errors.New("unable to locate cache directory (XDG_CACHE_HOME and HOME unset)")
	}

	cachedir := ""
	if xdgcache != "" {
		cachedir = filepath.Join(xdgcache, "restic")
	} else if home != "" {
		cachedir = filepath.Join(home, ".cache", "restic")
	}

	fi, err := fs.Stat(cachedir)
	if os.IsNotExist(errors.Cause(err)) {
		err = fs.MkdirAll(cachedir, 0700)
		if err != nil {
			return "", errors.Wrap(err, "MkdirAll")
		}

		fi, err = fs.Stat(cachedir)
		debug.Log("getCacheDir", "create cache dir %v", cachedir)
	}

	if err != nil {
		return "", errors.Wrap(err, "Stat")
	}

	if !fi.IsDir() {
		return "", errors.Errorf("cache dir %v is not a directory", cachedir)
	}

	return cachedir, nil
}

// New creates a new cache in the given directory. If it is the empty
// string, the cache directory for the current user is used instead.
func New(dir, repoID string) (cache restic.Cache, err error) {
	if dir == "" {
		dir, err = getXDGCacheDir()
		if err != nil {
			return nil, err
		}
	}

	if repoID == "" {
		return nil, errors.New("cache: empty repo id")
	}

	dir = filepath.Join(dir, repoID)

	return &Cache{dir: dir}, nil
}

func fn(dir string, h restic.BlobHandle) string {
	id := h.ID.String()
	subdir := id[:2]
	return filepath.Join(dir, h.Type.String(), subdir, id)
}

// GetBlob returns a blob from the cache. If the blob is not in the cache, ok
// is set to false.
func (c *Cache) GetBlob(h restic.BlobHandle, buf []byte) (ok bool, err error) {
	filename := fn(c.dir, h)

	fi, err := os.Stat(filename)
	if os.IsNotExist(errors.Cause(err)) {
		return false, nil
	}

	if fi.Size() != int64(len(buf)) {
		return false, errors.Errorf("wrong bufsize: %d != %d", fi.Size(), len(buf))
	}

	if err != nil {
		return false, errors.Wrap(err, "Stat")
	}

	var f *os.File
	f, err = os.Open(filename)
	if err != nil {
		return false, errors.Wrap(err, "Open")
	}

	defer func() {
		e := f.Close()
		if err == nil {
			err = e
		}
	}()

	_, err = io.ReadFull(f, buf)
	if err != nil {
		return false, err
	}

	return true, nil
}

func createDirs(filename string) error {
	dir := filepath.Dir(filename)
	fi, err := os.Stat(dir)
	if err != nil && os.IsNotExist(errors.Cause(err)) {
		err = os.MkdirAll(dir, 0700)
		return errors.Wrap(err, "mkdir cache dir")
	}

	if err != nil {
		return err
	}

	if !fi.IsDir() {
		return errors.Errorf("is not a directory: %v", dir)
	}

	return nil
}

// PutBlob saves a blob in the cache.
func (c *Cache) PutBlob(h restic.BlobHandle, buf []byte) error {
	filename := fn(c.dir, h)

	if err := createDirs(filename); err != nil {
		return err
	}

	return ioutil.WriteFile(filename, buf, 0600)
}

// DeleteBlob removes a blob from the cache. If it isn't included in the cache,
// a nil error is returned.
func (c *Cache) DeleteBlob(h restic.BlobHandle) error {
	err := os.Remove(fn(c.dir, h))
	if err != nil && os.IsNotExist(errors.Cause(err)) {
		err = nil
	}
	return err
}

// HasBlob check whether the cache has a particular blob.
func (c *Cache) HasBlob(h restic.BlobHandle) bool {
	_, err := os.Stat(fn(c.dir, h))
	if err != nil {
		return false
	}

	return true
}

func (c *Cache) updateBlobs(idx restic.BlobIndex, t restic.BlobType) (err error) {
	dir := filepath.Dir(fn(c.dir, restic.BlobHandle{Type: t}))

	var d *os.File
	d, err = os.Open(dir)
	if err != nil && os.IsNotExist(errors.Cause(err)) {
		return nil
	}

	if err != nil {
		return errors.Wrap(err, "Open")
	}

	defer func() {
		e := d.Close()
		if err == nil {
			err = errors.Wrap(e, "Close")
		}
	}()

	entries, err := d.Readdirnames(-1)
	if err != nil {
		return errors.Wrap(err, "Readdirnames")
	}

	debug.Log("Cache.UpdateBlobs", "checking %v/%d entries", t, len(entries))
	for _, name := range entries {
		id, err := restic.ParseID(name)
		if err != nil {
			debug.Log("Cache.UpdateBlobs", "  cache entry %q does not parse as id: %v", name, err)
			continue
		}

		if !idx.Has(id, t) {
			debug.Log("Cache.UpdateBlobs", "  remove %v/%v", t, name)

			err = os.Remove(filepath.Join(dir, name))
			if err != nil {
				return errors.Wrap(err, "Remove")
			}
		}
	}

	return nil
}

// UpdateBlobs takes an index and removes blobs from the local cache which are
// not in the repo any more.
func (c *Cache) UpdateBlobs(idx restic.BlobIndex) (err error) {
	err = c.updateBlobs(idx, restic.TreeBlob)
	if err != nil {
		return err
	}

	return c.updateBlobs(idx, restic.DataBlob)
}
