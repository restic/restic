package restic

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/restic/restic/debug"
)

// GetCacheDir returns the cache directory according to XDG basedir spec, see
// http://standards.freedesktop.org/basedir-spec/basedir-spec-latest.html
func GetCacheDir() (string, error) {
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
