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
	blobdir string
	filedir string
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

	c := &Cache{
		blobdir: filepath.Join(dir, "blob"),
		filedir: filepath.Join(dir, "file"),
	}

	return c, nil
}

func (c *Cache) get(filename string, buf []byte) (ok bool, err error) {
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

func (c *Cache) blobFn(h restic.BlobHandle) string {
	id := h.ID.String()
	subdir := id[:2]
	return filepath.Join(c.blobdir, h.Type.String(), subdir, id)
}

// GetBlob returns a blob from the cache. If the blob is not in the cache, ok
// is set to false.
func (c *Cache) GetBlob(h restic.BlobHandle, buf []byte) (ok bool, err error) {
	filename := c.blobFn(h)
	return c.get(filename, buf)
}

// PutBlob saves a blob in the cache.
func (c *Cache) PutBlob(h restic.BlobHandle, buf []byte) error {
	filename := c.blobFn(h)

	if err := createDirs(filename); err != nil {
		return err
	}

	return ioutil.WriteFile(filename, buf, 0600)
}

// DeleteBlob removes a blob from the cache. If it isn't included in the cache,
// a nil error is returned.
func (c *Cache) DeleteBlob(h restic.BlobHandle) error {
	err := os.Remove(c.blobFn(h))
	if err != nil && os.IsNotExist(errors.Cause(err)) {
		err = nil
	}
	return err
}

// HasBlob check whether the cache has a particular blob.
func (c *Cache) HasBlob(h restic.BlobHandle) bool {
	_, err := os.Stat(c.blobFn(h))
	if err != nil {
		return false
	}

	return true
}

func isFile(fi os.FileInfo) bool {
	return fi.Mode()&(os.ModeType|os.ModeCharDevice) == 0
}

func listDir(dir string) (entries []string, err error) {
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if info == nil {
			return nil
		}

		if isFile(info) {
			p, err := filepath.Rel(dir, path)
			if err != nil {
				return errors.Wrap(err, "filepath.Rel")
			}
			entries = append(entries, p)
		}

		return nil
	})

	if err != nil {
		return nil, errors.Wrap(err, "Walk")
	}

	return entries, nil
}

func (c *Cache) updateBlobs(idx restic.BlobIndex, t restic.BlobType) (err error) {
	basedir := filepath.Join(c.blobdir, t.String())

	entries, err := listDir(basedir)
	if err != nil {
		return err
	}

	debug.Log("Cache.UpdateBlobs", "checking %v/%d entries", t, len(entries))
	for _, path := range entries {
		name := filepath.Base(path)
		id, err := restic.ParseID(name)
		if err != nil {
			debug.Log("Cache.UpdateBlobs", "  cache entry %q does not parse as id: %v", name, err)
			continue
		}

		if !idx.Has(id, t) {
			debug.Log("Cache.UpdateBlobs", "  remove %v/%v", t, name)
			err = os.Remove(c.blobFn(restic.BlobHandle{Type: t, ID: id}))
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

func (c *Cache) fileFn(h restic.Handle) string {
	id := h.Name
	subdir := id[:2]
	return filepath.Join(c.filedir, h.Type.String(), subdir, id)
}

// GetFile returns a file from the cache. If the file is not in the cache, ok
// is set to false.
func (c *Cache) GetFile(h restic.Handle, buf []byte) (ok bool, err error) {
	filename := c.fileFn(h)
	return c.get(filename, buf)
}

var allowedFileTypes = map[restic.FileType]struct{}{
	restic.SnapshotFile: struct{}{},
	restic.IndexFile:    struct{}{},
}

// PutFile saves a file in the cache.
func (c *Cache) PutFile(h restic.Handle, buf []byte) error {
	if _, ok := allowedFileTypes[h.Type]; !ok {
		return errors.Errorf("filetype %v not allowed for cache", h.Type)
	}

	filename := c.fileFn(h)

	if err := createDirs(filename); err != nil {
		return err
	}

	return ioutil.WriteFile(filename, buf, 0600)
}

// DeleteFile removes a file from the cache. If it isn't included in the cache,
// a nil error is returned.
func (c *Cache) DeleteFile(h restic.Handle) error {
	err := os.Remove(c.fileFn(h))
	if err != nil && os.IsNotExist(errors.Cause(err)) {
		err = nil
	}
	return err
}

// HasFile check whether the cache has a particular file.
func (c *Cache) HasFile(h restic.Handle) bool {
	_, err := os.Stat(c.fileFn(h))
	if err != nil {
		return false
	}

	return true
}

func (c *Cache) updateFiles(idx restic.FileIndex, t restic.FileType) (err error) {
	entries, err := listDir(filepath.Join(c.filedir, t.String()))
	if err != nil {
		return err
	}

	debug.Log("Cache.UpdateFiles", "checking %v/%d entries", t, len(entries))
	for _, path := range entries {
		name := filepath.Base(path)
		ok, err := idx.Test(t, name)
		if err != nil {
			return errors.Wrap(err, "Test")
		}

		if !ok {
			debug.Log("Cache.UpdateFiles", "  remove %v/%v", t, name)

			h := restic.Handle{Name: name, Type: t}
			err = os.Remove(c.fileFn(h))
			if err != nil {
				return errors.Wrap(err, "Remove")
			}
		}
	}

	return nil
}

// UpdateFiles takes an index and removes files from the local cache which are
// not in the repo any more.
func (c *Cache) UpdateFiles(idx restic.FileIndex) (err error) {
	for t := range allowedFileTypes {
		err = c.updateFiles(idx, t)
		if err != nil {
			return err
		}
	}

	return nil
}
