package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
)

// Cache manages a local cache.
type Cache struct {
	path    string
	Base    string
	Created bool
}

const dirMode = 0700
const fileMode = 0644

func readVersion(dir string) (v uint, err error) {
	buf, err := os.ReadFile(filepath.Join(dir, "version"))
	if err != nil {
		return 0, errors.Wrap(err, "readVersion")
	}

	ver, err := strconv.ParseUint(string(buf), 10, 32)
	if err != nil {
		return 0, errors.Wrap(err, "readVersion")
	}

	return uint(ver), nil
}

const cacheVersion = 1

var cacheLayoutPaths = map[restic.FileType]string{
	restic.PackFile:     "data",
	restic.SnapshotFile: "snapshots",
	restic.IndexFile:    "index",
}

const cachedirTagSignature = "Signature: 8a477f597d28d172789f06886806bc55\n"

func writeCachedirTag(dir string) error {
	tagfile := filepath.Join(dir, "CACHEDIR.TAG")
	f, err := fs.OpenFile(tagfile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, fileMode)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}

		return errors.WithStack(err)
	}

	debug.Log("Create CACHEDIR.TAG at %v", dir)
	if _, err := f.Write([]byte(cachedirTagSignature)); err != nil {
		_ = f.Close()
		return errors.WithStack(err)
	}

	return errors.WithStack(f.Close())
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

	err = fs.MkdirAll(basedir, dirMode)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// create base dir and tag it as a cache directory
	if err = writeCachedirTag(basedir); err != nil {
		return nil, err
	}

	cachedir := filepath.Join(basedir, id)
	debug.Log("using cache dir %v", cachedir)

	created := false
	v, err := readVersion(cachedir)
	switch {
	case err == nil:
		if v > cacheVersion {
			return nil, errors.New("cache version is newer")
		}
		// Update the timestamp so that we can detect old cache dirs.
		err = updateTimestamp(cachedir)
		if err != nil {
			return nil, err
		}

	case errors.Is(err, os.ErrNotExist):
		// Create the repo cache dir. The parent exists, so Mkdir suffices.
		err := fs.Mkdir(cachedir, dirMode)
		switch {
		case err == nil:
			created = true
		case errors.Is(err, os.ErrExist):
		default:
			return nil, errors.WithStack(err)
		}

	default:
		return nil, errors.Wrap(err, "readVersion")
	}

	if v < cacheVersion {
		err = os.WriteFile(filepath.Join(cachedir, "version"), []byte(fmt.Sprintf("%d", cacheVersion)), fileMode)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	for _, p := range cacheLayoutPaths {
		if err = fs.MkdirAll(filepath.Join(cachedir, p), dirMode); err != nil {
			return nil, errors.WithStack(err)
		}
	}

	c = &Cache{
		path:    cachedir,
		Base:    basedir,
		Created: created,
	}

	return c, nil
}

// updateTimestamp sets the modification timestamp (mtime and atime) for the
// directory d to the current time.
func updateTimestamp(d string) error {
	t := time.Now()
	return fs.Chtimes(d, t, t)
}

// MaxCacheAge is the default age (30 days) after which cache directories are considered old.
const MaxCacheAge = 30 * 24 * time.Hour

func validCacheDirName(s string) bool {
	r := regexp.MustCompile(`^[a-fA-F0-9]{64}$|^restic-check-cache-[0-9]+$`)
	return r.MatchString(s)
}

// listCacheDirs returns the list of cache directories.
func listCacheDirs(basedir string) ([]os.FileInfo, error) {
	f, err := fs.Open(basedir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			err = nil
		}
		return nil, err
	}

	entries, err := f.Readdir(-1)
	if err != nil {
		return nil, err
	}

	err = f.Close()
	if err != nil {
		return nil, err
	}

	result := make([]os.FileInfo, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		if !validCacheDirName(entry.Name()) {
			continue
		}

		result = append(result, entry)
	}

	return result, nil
}

// All returns a list of cache directories.
func All(basedir string) (dirs []os.FileInfo, err error) {
	return listCacheDirs(basedir)
}

// OlderThan returns the list of cache directories older than max.
func OlderThan(basedir string, max time.Duration) ([]os.FileInfo, error) {
	entries, err := listCacheDirs(basedir)
	if err != nil {
		return nil, err
	}

	var oldCacheDirs []os.FileInfo
	for _, fi := range entries {
		if !IsOld(fi.ModTime(), max) {
			continue
		}

		oldCacheDirs = append(oldCacheDirs, fi)
	}

	debug.Log("%d old cache dirs found", len(oldCacheDirs))

	return oldCacheDirs, nil
}

// Old returns a list of cache directories with a modification time of more
// than 30 days ago.
func Old(basedir string) ([]os.FileInfo, error) {
	return OlderThan(basedir, MaxCacheAge)
}

// IsOld returns true if the timestamp is considered old.
func IsOld(t time.Time, maxAge time.Duration) bool {
	oldest := time.Now().Add(-maxAge)
	return t.Before(oldest)
}

// Wrap returns a backend with a cache.
func (c *Cache) Wrap(be restic.Backend) restic.Backend {
	return newBackend(be, c)
}

// BaseDir returns the base directory.
func (c *Cache) BaseDir() string {
	return c.Base
}
