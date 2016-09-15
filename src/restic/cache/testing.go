package cache

import (
	"path/filepath"
	"restic"
	"restic/test"
	"testing"
)

// TestNewCache creates a cache usable for testing.
func TestNewCache(t testing.TB) (restic.Cache, func()) {
	tempdir, cleanup := test.TempDir(t)

	cachedir := filepath.Join(tempdir, "cache")
	c, err := New(cachedir, "test")
	test.OK(t, err)

	return c, cleanup
}
