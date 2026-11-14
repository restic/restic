package local

import (
	"os"
	"path/filepath"
	"testing"
)

// fsyncDir must flush a real directory without error. A regression that opens
// the handle without GENERIC_WRITE fails here with ERROR_ACCESS_DENIED.
func TestFSyncDirFlushesDirectory(t *testing.T) {
	dir := t.TempDir()

	if err := fsyncDir(dir); err != nil {
		t.Fatalf("fsyncDir(%q) returned error: %v", dir, err)
	}
}

// A missing directory must surface an error, not be silently skipped.
func TestFSyncDirErrorsOnMissingDirectory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	if err := fsyncDir(missing); err == nil {
		t.Fatalf("fsyncDir(%q) returned nil error for missing directory", missing)
	}
}

// BenchmarkFSyncDirAfterSave measures the per-file cost restic adds: after
// every saved file, Save calls fsyncDir on the (now dirty) directory, the
// same way it already does on POSIX.
func BenchmarkFSyncDirAfterSave(b *testing.B) {
	dir := b.TempDir()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		name := filepath.Join(dir, "file")
		if err := os.WriteFile(name, []byte("x"), 0o666); err != nil {
			b.Fatal(err)
		}
		if err := fsyncDir(dir); err != nil {
			b.Fatal(err)
		}
		if err := os.Remove(name); err != nil {
			b.Fatal(err)
		}
	}
}
