package cache

import (
	"fmt"
	"os"
	"path/filepath"
)

// EnvDir return $RESTIC_CACHE_DIR env
func EnvDir() string {
	return os.Getenv("RESTIC_CACHE_DIR")
}

// DefaultDir returns $RESTIC_CACHE_DIR, or the default cache directory
// for the current OS if that variable is not set.
func DefaultDir() (cachedir string, err error) {
	cachedir = EnvDir()
	if cachedir != "" {
		return cachedir, nil
	}

	cachedir, err = os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("unable to locate cache directory: %v", err)
	}

	return filepath.Join(cachedir, "restic"), nil
}
