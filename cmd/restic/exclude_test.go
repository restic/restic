package main

import (
	"path/filepath"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func TestRejectResticTempDir(t *testing.T) {
	parent := t.TempDir()
	tempDir := filepath.Join(parent, "restic-temp")
	reject := rejectResticTempDir(tempDir)

	for _, path := range []string{
		tempDir,
		filepath.Join(tempDir, "restic-temp-pack-123"),
		filepath.Join(tempDir, "subdir", "file"),
	} {
		rtest.Assert(t, reject(path), "expected %q to be rejected", path)
	}

	for _, path := range []string{
		parent,
		filepath.Join(parent, "restic-temp-sibling"),
		filepath.Join(parent, "other", "file"),
	} {
		rtest.Assert(t, !reject(path), "did not expect %q to be rejected", path)
	}
}
