// Package cache implements a local cache for data.
package cache

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"restic"
	"restic/errors"
)

// Cache is a local cache implementation.
type Cache struct {
	dir string
}

// make sure that Cache implement restic.Cache
var _ restic.Cache = &Cache{}

// NewCache creates a new cache in the given directory. If it is the empty
// string, the cache directory for the current user is used instead.
func NewCache(dir string) restic.Cache {
	return &Cache{dir: dir}
}

func fn(dir string, h restic.BlobHandle) string {
	return filepath.Join(dir, string(h.Type), h.ID.String())
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

	return ioutil.WriteFile(filename, buf, 0700)
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
