package cache

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
)

// Cache manages a local cache.
type Cache struct {
	Path             string
	Base             string
	PerformReadahead func(restic.Handle) bool
}

const dirMode = 0700
const fileMode = 0600

func readVersion(dir string) (v uint, err error) {
	buf, err := ioutil.ReadFile(filepath.Join(dir, "version"))
	if os.IsNotExist(err) {
		return 0, nil
	}

	if err != nil {
		return 0, errors.Wrap(err, "ReadFile")
	}

	ver, err := strconv.ParseUint(string(buf), 10, 32)
	if err != nil {
		return 0, errors.Wrap(err, "ParseUint")
	}

	return uint(ver), nil
}

const cacheVersion = 1

// ensure Cache implements restic.Cache
var _ restic.Cache = &Cache{}

var cacheLayoutPaths = map[restic.FileType]string{
	restic.DataFile:     "data",
	restic.SnapshotFile: "snapshots",
	restic.IndexFile:    "index",
}

const cachedirTagSignature = "Signature: 8a477f597d28d172789f06886806bc55\n"

func writeCachedirTag(dir string) error {
	if err := fs.MkdirAll(dir, dirMode); err != nil {
		return err
	}

	f, err := fs.OpenFile(filepath.Join(dir, "CACHEDIR.TAG"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		if os.IsExist(errors.Cause(err)) {
			return nil
		}

		return errors.Wrap(err, "OpenFile")
	}

	debug.Log("Create CACHEDIR.TAG at %v", dir)
	if _, err := f.Write([]byte(cachedirTagSignature)); err != nil {
		_ = f.Close()
		return errors.Wrap(err, "Write")
	}

	return f.Close()
}

// New returns a new cache for the repo ID at basedir. If basedir is the empty
// string, the default cache location (according to the XDG standard) is used.
//
// For partial files, the complete file is loaded and stored in the cache when
// performReadahead returns true.
func New(id string, basedir string) (c *Cache, err error) {
	if basedir == "" {
		basedir, err = DefaultDir()
		if err != nil {
			return nil, err
		}
	}

	err = mkdirCacheDir(basedir)
	if err != nil {
		return nil, err
	}

	// create base dir and tag it as a cache directory
	if err = writeCachedirTag(basedir); err != nil {
		return nil, err
	}

	cachedir := filepath.Join(basedir, id)
	debug.Log("using cache dir %v", cachedir)

	v, err := readVersion(cachedir)
	if err != nil {
		return nil, err
	}

	if v > cacheVersion {
		return nil, errors.New("cache version is newer")
	}

	// create the repo cache dir if it does not exist yet
	if err = fs.MkdirAll(cachedir, dirMode); err != nil {
		return nil, err
	}

	// update the timestamp so that we can detect old cache dirs
	err = updateTimestamp(cachedir)
	if err != nil {
		return nil, err
	}

	if v < cacheVersion {
		err = ioutil.WriteFile(filepath.Join(cachedir, "version"), []byte(fmt.Sprintf("%d", cacheVersion)), 0644)
		if err != nil {
			return nil, errors.Wrap(err, "WriteFile")
		}
	}

	for _, p := range cacheLayoutPaths {
		if err = fs.MkdirAll(filepath.Join(cachedir, p), dirMode); err != nil {
			return nil, err
		}
	}

	c = &Cache{
		Path: cachedir,
		Base: basedir,
		PerformReadahead: func(restic.Handle) bool {
			// do not perform readahead by default
			return false
		},
	}

	return c, nil
}

// updateTimestamp sets the modification timestamp (mtime and atime) for the
// directory d to the current time.
func updateTimestamp(d string) error {
	t := time.Now()
	return fs.Chtimes(d, t, t)
}

const maxCacheAge = 30 * 24 * time.Hour

// Old returns a list of cache directories with a modification time of more
// than 30 days ago.
func Old(basedir string) ([]string, error) {
	var oldCacheDirs []string

	f, err := fs.Open(basedir)
	if err != nil {
		return nil, err
	}

	entries, err := f.Readdir(-1)
	if err != nil {
		return nil, err
	}

	oldest := time.Now().Add(-maxCacheAge)
	for _, fi := range entries {
		if !fi.IsDir() {
			continue
		}

		if !fi.ModTime().Before(oldest) {
			continue
		}

		oldCacheDirs = append(oldCacheDirs, fi.Name())
	}

	err = f.Close()
	if err != nil {
		return nil, err
	}

	debug.Log("%d old cache dirs found", len(oldCacheDirs))

	return oldCacheDirs, nil
}

// errNoSuchFile is returned when a file is not cached.
type errNoSuchFile struct {
	Type string
	Name string
}

func (e errNoSuchFile) Error() string {
	return fmt.Sprintf("file %v (%v) is not cached", e.Name, e.Type)
}

// IsNotExist returns true if the error was caused by a non-existing file.
func (c *Cache) IsNotExist(err error) bool {
	_, ok := errors.Cause(err).(errNoSuchFile)
	return ok
}

// Wrap returns a backend with a cache.
func (c *Cache) Wrap(be restic.Backend) restic.Backend {
	return newBackend(be, c)
}

// BaseDir returns the base directory.
func (c *Cache) BaseDir() string {
	return c.Base
}
