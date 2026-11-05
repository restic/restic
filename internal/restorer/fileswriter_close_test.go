package restorer

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func closeErrorTestFile(t *testing.T, name string) *os.File {
	t.Helper()
	f, err := os.Create(filepath.Join(t.TempDir(), name))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestFilesWriterFlushReturnsCloseError(t *testing.T) {
	w := newFilesWriter(1, false)
	f := closeErrorTestFile(t, "already-closed")
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	w.cacheMu.Lock()
	w.cache.Add(f.Name(), &partialFile{File: f})
	w.cacheMu.Unlock()

	if err := w.flush(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("flush() = %v, want os.ErrClosed from cached handle", err)
	}
	// A reported failure must not recur on a later healthy flush.
	if err := w.flush(); err != nil {
		t.Fatalf("second flush() = %v, want nil", err)
	}
}

func TestFilesWriterReportsEvictionCloseErrorAtFlush(t *testing.T) {
	w := newFilesWriter(0, false)
	bad := closeErrorTestFile(t, "evicted-closed")
	if err := bad.Close(); err != nil {
		t.Fatal(err)
	}
	w.cacheMu.Lock()
	w.cache.Add(bad.Name(), &partialFile{File: bad})
	for i := 0; i < 60; i++ {
		f := closeErrorTestFile(t, "healthy")
		w.cache.Add(f.Name(), &partialFile{File: f})
	}
	w.cacheMu.Unlock()

	if err := w.flush(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("flush() = %v, want earlier eviction os.ErrClosed", err)
	}
}

func TestFilesWriterHealthyFlushReturnsNilAndPreservesContents(t *testing.T) {
	w := newFilesWriter(1, false)
	target := filepath.Join(t.TempDir(), "restored-file")
	contents := []byte("safe-data")
	if err := w.writeToFile(target, contents, 0, int64(len(contents)), false); err != nil {
		t.Fatal(err)
	}
	if err := w.flush(); err != nil {
		t.Fatalf("flush() = %v, want nil", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, contents) {
		t.Fatalf("restored contents = %q, want %q", got, contents)
	}
}

func TestRestorerPropagatesFileCloseFailure(t *testing.T) {
	w := newFilesWriter(1, false)
	f := closeErrorTestFile(t, "closed-before-flush")
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	w.cacheMu.Lock()
	w.cache.Add(f.Name(), &partialFile{File: f})
	w.cacheMu.Unlock()

	r := &fileRestorer{filesWriter: w}
	if err := r.restoreFiles(context.Background()); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("restoreFiles() = %v, want file close error", err)
	}
}
