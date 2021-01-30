package cache

import (
	"os"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

// DefaultDir should honor RESTIC_CACHE_DIR on all platforms.
func TestCacheDirEnv(t *testing.T) {
	cachedir := os.Getenv("RESTIC_CACHE_DIR")

	if cachedir == "" {
		cachedir = "/doesnt/exist"
		err := os.Setenv("RESTIC_CACHE_DIR", cachedir)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			err := os.Unsetenv("RESTIC_CACHE_DIR")
			if err != nil {
				t.Fatal(err)
			}
		}()
	}

	dir, err := DefaultDir()
	rtest.Equals(t, cachedir, dir)
	rtest.OK(t, err)
}
