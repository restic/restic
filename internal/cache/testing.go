package cache

import (
	"testing"

	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
)

// TestNewCache returns a cache in a temporary directory which is removed when
// cleanup is called.
func TestNewCache(t testing.TB) (*Cache, func()) {
	dir, cleanup := test.TempDir(t)
	t.Logf("created new cache at %v", dir)
	cache, err := New(restic.NewRandomID().String(), dir, LayoutStandard)
	if err != nil {
		t.Fatal(err)
	}
	return cache, cleanup
}

// TestNewCacheAll returns a cache where all files are cached in a temporary
// directory which is removed when cleanup is called.
func TestNewCacheAll(t testing.TB) (*Cache, func()) {
	dir, cleanup := test.TempDir(t)
	t.Logf("created new cache at %v", dir)
	cache, err := New("repo-"+restic.NewRandomID().String(), dir, LayoutAll)
	if err != nil {
		t.Fatal(err)
	}
	return cache, cleanup
}
