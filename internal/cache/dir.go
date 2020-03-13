package cache

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/fs"
)

// DefaultDir returns $RESTIC_CACHE_DIR, or the default cache directory
// for the current OS if that variable is not set.
func DefaultDir() (cachedir string, err error) {
	cachedir = os.Getenv("RESTIC_CACHE_DIR")
	if cachedir != "" {
		return cachedir, nil
	}

	cachedir, err = os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("unable to locate cache directory: %v", err)
	}

	return filepath.Join(cachedir, "restic"), nil
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
