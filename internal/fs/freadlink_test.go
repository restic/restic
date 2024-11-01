//go:build linux || windows || darwin
// +build linux windows darwin

package fs

import (
	"os"
	"path/filepath"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func TestFreadlink(t *testing.T) {
	tmpdir := t.TempDir()
	link := filepath.Join(tmpdir, "link")
	rtest.OK(t, os.Symlink("other", link))

	f, err := openMetadataHandle(link, O_NOFOLLOW)
	rtest.OK(t, err)
	target, err := Freadlink(f.Fd(), link)
	rtest.OK(t, err)
	rtest.Equals(t, "other", target)
}
