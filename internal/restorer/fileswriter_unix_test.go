//go:build unix

package restorer

import (
	"fmt"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

// closeCachedFile closes the file descriptor of the cached handle for path,
// so that the next Close of the handle fails.
func closeCachedFile(t *testing.T, w *filesWriter, path string) {
	t.Helper()
	wr, ok := w.cache.Peek(path)
	rtest.Assert(t, ok, "handle for %q not cached", path)
	rtest.OK(t, syscall.Close(int(wr.Fd())))
}

func TestFilesWriterFlushReportsCloseError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	w := newFilesWriter(1, false)
	rtest.OK(t, w.writeToFile(path, []byte("data"), 0, 4, false))

	closeCachedFile(t, w, path)

	err := w.flush(nil)
	rtest.Assert(t, err != nil, "expected an error from flush, got nil")
	rtest.Assert(t, strings.Contains(err.Error(), path), "error should mention the file, got %v", err)

	// reported errors are cleared, a second flush has nothing to report
	rtest.OK(t, w.flush(nil))
}

func TestFilesWriterFlushReportsEvictedCloseError(t *testing.T) {
	dir := t.TempDir()
	// the cache holds count+50 handles, so writing this many files
	// evicts the first one while restoring
	w := newFilesWriter(1, false)

	first := filepath.Join(dir, "first")
	rtest.OK(t, w.writeToFile(first, []byte("data"), 0, 4, false))

	// close the cached handle directly: closing the raw file descriptor
	// instead would let a later file reuse the descriptor number, and
	// the eviction would then close that file instead
	wr, ok := w.cache.Peek(first)
	rtest.Assert(t, ok, "handle for %q not cached", first)
	rtest.OK(t, wr.Close())

	for i := 0; i < 51; i++ {
		path := filepath.Join(dir, fmt.Sprintf("f%d", i))
		rtest.OK(t, w.writeToFile(path, []byte("data"), 0, 4, false))
	}

	// the first file was evicted while its close failed, the error
	// must still be reported by flush
	err := w.flush(nil)
	rtest.Assert(t, err != nil, "expected an error from flush, got nil")
	rtest.Assert(t, strings.Contains(err.Error(), first), "error should mention the evicted file, got %v", err)
}

func TestFilesWriterFlushWithoutCloseError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	w := newFilesWriter(1, false)
	rtest.OK(t, w.writeToFile(path, []byte("data"), 0, 4, false))
	rtest.OK(t, w.flush(nil))
}

func TestFilesWriterFlushFilterDropsCloseError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	w := newFilesWriter(1, false)
	rtest.OK(t, w.writeToFile(path, []byte("data"), 0, 4, false))

	closeCachedFile(t, w, path)

	var filteredPaths []string
	filter := func(filterPath string, err error) error {
		rtest.Assert(t, err != nil, "filter got nil error for %q", filterPath)
		filteredPaths = append(filteredPaths, filterPath)
		// drop the error after reporting it, as the restore command does
		return nil
	}

	// a filter that drops the error must keep flush from reporting it
	rtest.OK(t, w.flush(filter))
	rtest.Equals(t, []string{path}, filteredPaths)

	// reported errors are cleared, a second flush has nothing to filter
	rtest.OK(t, w.flush(filter))
	rtest.Equals(t, 1, len(filteredPaths))
}

func TestFilesWriterFlushFilterDropsEvictedCloseError(t *testing.T) {
	dir := t.TempDir()
	// the cache holds count+50 handles, so writing this many files
	// evicts the first one while restoring
	w := newFilesWriter(1, false)

	first := filepath.Join(dir, "first")
	rtest.OK(t, w.writeToFile(first, []byte("data"), 0, 4, false))

	// close the cached handle directly: closing the raw file descriptor
	// instead would let a later file reuse the descriptor number, and
	// the eviction would then close that file instead
	wr, ok := w.cache.Peek(first)
	rtest.Assert(t, ok, "handle for %q not cached", first)
	rtest.OK(t, wr.Close())

	for i := 0; i < 51; i++ {
		path := filepath.Join(dir, fmt.Sprintf("f%d", i))
		rtest.OK(t, w.writeToFile(path, []byte("data"), 0, 4, false))
	}

	// the close error of the evicted file must be passed through the
	// filter as well, and dropped if the filter drops it
	var filteredPaths []string
	filter := func(filterPath string, err error) error {
		rtest.Assert(t, err != nil, "filter got nil error for %q", filterPath)
		filteredPaths = append(filteredPaths, filterPath)
		return nil
	}
	rtest.OK(t, w.flush(filter))
	rtest.Equals(t, []string{first}, filteredPaths)
}

func TestFilesWriterFlushFilterKeepsCloseError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	w := newFilesWriter(1, false)
	rtest.OK(t, w.writeToFile(path, []byte("data"), 0, 4, false))

	closeCachedFile(t, w, path)

	// a filter that returns the error, as with the default error handling,
	// must let flush report it, mentioning the file
	err := w.flush(func(_ string, err error) error { return err })
	rtest.Assert(t, err != nil, "expected an error from flush, got nil")
	rtest.Assert(t, strings.Contains(err.Error(), path), "error should mention the file, got %v", err)
}
