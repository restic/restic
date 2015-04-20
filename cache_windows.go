package restic

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/restic/restic/debug"
)

// GetCacheDir returns the cache directory
func GetCacheDir() (string, error) {
	AppData := os.Getenv("AppData")

	if AppData == "" {
		return "", errors.New("unable to locate cache directory")
	}

	cachedir := ""
	if AppData != "" {
		cachedir = filepath.Join(AppData, "restic")
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
