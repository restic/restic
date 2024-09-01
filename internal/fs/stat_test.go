package fs

import (
	"os"
	"path/filepath"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func TestExtendedStat(t *testing.T) {
	tempdir := rtest.TempDir(t)
	filename := filepath.Join(tempdir, "file")
	err := os.WriteFile(filename, []byte("foobar"), 0640)
	if err != nil {
		t.Fatal(err)
	}

	fi, err := Lstat(filename)
	if err != nil {
		t.Fatal(err)
	}

	extFI := ExtendedStat(fi)

	if !extFI.ModTime.Equal(fi.ModTime()) {
		t.Errorf("extFI.ModTime does not match, want %v, got %v", fi.ModTime(), extFI.ModTime)
	}
}
