package cache

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/fs"
)

// xdgCacheDir returns the cache directory according to XDG basedir spec, see
// http://standards.freedesktop.org/basedir-spec/basedir-spec-latest.html
func xdgCacheDir() (string, error) {
	xdgcache := os.Getenv("XDG_CACHE_HOME")
	home := os.Getenv("HOME")

	if xdgcache != "" {
		return filepath.Join(xdgcache, "restic"), nil
	} else if home != "" {
		return filepath.Join(home, ".cache", "restic"), nil
	}

	return "", errors.New("unable to locate cache directory (XDG_CACHE_HOME and HOME unset)")
}

// windowsCacheDir returns the cache directory for Windows.
//
// Uses LOCALAPPDATA, where application data not synchronized between machines
// is stored. (Browser caches stored here).
func windowsCacheDir() (string, error) {
	appdata := os.Getenv("LOCALAPPDATA")
	if appdata == "" {
		return "", errors.New("unable to locate cache directory (LOCALAPPDATA unset)")
	}
	return filepath.Join(appdata, "restic"), nil
}

// darwinCacheDir returns the cache directory for darwin.
//
// Uses ~/Library/Caches/, which is recommended by Apple, see
// https://developer.apple.com/library/content/documentation/FileManagement/Conceptual/FileSystemProgrammingGuide/MacOSXDirectories/MacOSXDirectories.html
func darwinCacheDir() (string, error) {
	home := os.Getenv("HOME")
	if home == "" {
		return "", errors.New("unable to locate cache directory (HOME unset)")
	}
	return filepath.Join(home, "Library", "Caches", "restic"), nil
}

// DefaultDir returns the default cache directory for the current OS.
func DefaultDir() (cachedir string, err error) {
	switch runtime.GOOS {
	case "darwin":
		cachedir, err = darwinCacheDir()
	case "windows":
		cachedir, err = windowsCacheDir()
	default:
		// Default to XDG for Linux and any other OSes.
		cachedir, err = xdgCacheDir()
	}

	if err != nil {
		return "", err
	}

	return cachedir, nil
}

// mkdirCacheDir ensures that the cache directory exists. It it didn't, created
// is set to true.
func mkdirCacheDir(cachedir string) (created bool, err error) {
	var newCacheDir bool

	fi, err := fs.Stat(cachedir)
	if os.IsNotExist(errors.Cause(err)) {
		err = fs.MkdirAll(cachedir, 0700)
		if err != nil {
			return true, errors.Wrap(err, "MkdirAll")
		}

		fi, err = fs.Stat(cachedir)
		debug.Log("create cache dir %v", cachedir)

		newCacheDir = true
	}

	if err != nil {
		return newCacheDir, errors.Wrap(err, "Stat")
	}

	if !fi.IsDir() {
		return newCacheDir, errors.Errorf("cache dir %v is not a directory", cachedir)
	}

	return newCacheDir, nil
}
