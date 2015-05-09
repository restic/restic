package restic

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/debug"
	"github.com/restic/restic/repository"
)

// Cache is used to locally cache items from a repository.
type Cache struct {
	base string
}

func NewCache(repo *repository.Repository) (*Cache, error) {
	cacheDir, err := getCacheDir()
	if err != nil {
		return nil, err
	}

	basedir := filepath.Join(cacheDir, repo.Config.ID)
	debug.Log("Cache.New", "opened cache at %v", basedir)

	return &Cache{base: basedir}, nil
}

// Has checks if the local cache has the id.
func (c *Cache) Has(t backend.Type, subtype string, id backend.ID) (bool, error) {
	filename, err := c.filename(t, subtype, id)
	if err != nil {
		return false, err
	}
	fd, err := os.Open(filename)
	defer fd.Close()

	if err != nil {
		if os.IsNotExist(err) {
			debug.Log("Cache.Has", "test for file %v: not cached", filename)
			return false, nil
		}

		debug.Log("Cache.Has", "test for file %v: error %v", filename, err)
		return false, err
	}

	debug.Log("Cache.Has", "test for file %v: is cached", filename)
	return true, nil
}

// Store returns an io.WriteCloser that is used to save new information to the
// cache. The returned io.WriteCloser must be closed by the caller after all
// data has been written.
func (c *Cache) Store(t backend.Type, subtype string, id backend.ID) (io.WriteCloser, error) {
	filename, err := c.filename(t, subtype, id)
	if err != nil {
		return nil, err
	}

	dirname := filepath.Dir(filename)
	err = os.MkdirAll(dirname, 0700)
	if err != nil {
		return nil, err
	}

	file, err := os.Create(filename)
	if err != nil {
		debug.Log("Cache.Store", "error creating file %v: %v", filename, err)
		return nil, err
	}

	debug.Log("Cache.Store", "created file %v", filename)
	return file, nil
}

// Load returns information from the cache. The returned io.ReadCloser must be
// closed by the caller.
func (c *Cache) Load(t backend.Type, subtype string, id backend.ID) (io.ReadCloser, error) {
	filename, err := c.filename(t, subtype, id)
	if err != nil {
		return nil, err
	}

	return os.Open(filename)
}

func (c *Cache) purge(t backend.Type, subtype string, id backend.ID) error {
	filename, err := c.filename(t, subtype, id)
	if err != nil {
		return err
	}

	err = os.Remove(filename)
	debug.Log("Cache.purge", "Remove file %v: %v", filename, err)

	if err != nil && os.IsNotExist(err) {
		return nil
	}

	return err
}

// Clear removes information from the cache that isn't present in the repository any more.
func (c *Cache) Clear(repo *repository.Repository) error {
	list, err := c.list(backend.Snapshot)
	if err != nil {
		return err
	}

	for _, entry := range list {
		debug.Log("Cache.Clear", "found entry %v", entry)

		if ok, err := repo.Test(backend.Snapshot, entry.ID.String()); !ok || err != nil {
			debug.Log("Cache.Clear", "snapshot %v doesn't exist any more, removing %v", entry.ID, entry)

			err = c.purge(backend.Snapshot, entry.Subtype, entry.ID)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

type cacheEntry struct {
	ID      backend.ID
	Subtype string
}

func (c cacheEntry) String() string {
	if c.Subtype != "" {
		return c.ID.Str() + "." + c.Subtype
	}
	return c.ID.Str()
}

func (c *Cache) list(t backend.Type) ([]cacheEntry, error) {
	var dir string

	switch t {
	case backend.Snapshot:
		dir = filepath.Join(c.base, "snapshots")
	default:
		return nil, fmt.Errorf("cache not supported for type %v", t)
	}

	fd, err := os.Open(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []cacheEntry{}, nil
		}
		return nil, err
	}
	defer fd.Close()

	fis, err := fd.Readdir(-1)
	if err != nil {
		return nil, err
	}

	entries := make([]cacheEntry, 0, len(fis))

	for _, fi := range fis {
		parts := strings.SplitN(fi.Name(), ".", 2)

		id, err := backend.ParseID(parts[0])
		// ignore invalid cache entries for now
		if err != nil {
			debug.Log("Cache.List", "unable to parse name %v as id: %v", parts[0], err)
			continue
		}

		e := cacheEntry{ID: id}

		if len(parts) == 2 {
			e.Subtype = parts[1]
		}

		entries = append(entries, e)
	}

	return entries, nil
}

func (c *Cache) filename(t backend.Type, subtype string, id backend.ID) (string, error) {
	filename := id.String()
	if subtype != "" {
		filename += "." + subtype
	}

	switch t {
	case backend.Snapshot:
		return filepath.Join(c.base, "snapshots", filename), nil
	}

	return "", fmt.Errorf("cache not supported for type %v", t)
}

func getCacheDir() (string, error) {
	if dir := os.Getenv("RESTIC_CACHE"); dir != "" {
		return dir, nil
	}

	return getXDGCacheDir()
}

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

	fi, err := os.Stat(cachedir)
	if os.IsNotExist(err) {
		err = os.MkdirAll(cachedir, 0700)
		if err != nil {
			return "", err
		}

		fi, err = os.Stat(cachedir)
		debug.Log("getCacheDir", "create cache dir %v", cachedir)
	}

	if err != nil {
		return "", err
	}

	if !fi.IsDir() {
		return "", fmt.Errorf("cache dir %v is not a directory", cachedir)
	}

	return cachedir, nil
}
