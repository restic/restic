package fs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restic/restic/internal/data"
	rtest "github.com/restic/restic/internal/test"
	"golang.org/x/sys/windows"
)

func TestNodeCreateSymlinkWindowsPaths(t *testing.T) {
	for _, tc := range []struct {
		name           string
		long, prefixed bool
	}{
		{name: "relative parent"},
		{name: "long path", long: true},
		{name: "prefixed path", prefixed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			if tc.long {
				base = filepath.Join(base, strings.Repeat("long", 25), strings.Repeat("path", 25), strings.Repeat("name", 25))
			}
			parent := filepath.Join(base, "links")
			rtest.OK(t, os.MkdirAll(fixpath(parent), 0o700))
			rtest.OK(t, os.Mkdir(fixpath(filepath.Join(base, "directory")), 0o700))
			rtest.OK(t, os.WriteFile(fixpath(filepath.Join(base, "directory", "file")), []byte("content"), 0o600))
			linkPath := filepath.Join(parent, "link")
			if tc.prefixed {
				linkPath = extendedPathPrefix + linkPath
			}
			linkTarget := filepath.FromSlash("../directory")
			rtest.OK(t, nodeCreateSymlinkAt(&data.Node{LinkTarget: linkTarget}, linkPath))
			actual, err := os.Readlink(fixpath(linkPath))
			rtest.OK(t, err)
			rtest.Equals(t, linkTarget, actual)
			entries, err := os.ReadDir(fixpath(linkPath))
			rtest.OK(t, err)
			rtest.Equals(t, 1, len(entries))
			content, err := os.ReadFile(fixpath(filepath.Join(linkPath, "file")))
			rtest.OK(t, err)
			rtest.Equals(t, "content", string(content))
		})
	}
}

func TestNodeCreateSymlinkWindowsAbsoluteTarget(t *testing.T) {
	base := t.TempDir()
	rtest.OK(t, os.Mkdir(filepath.Join(base, "directory"), 0o700))
	rtest.OK(t, os.WriteFile(filepath.Join(base, "directory", "file"), []byte("content"), 0o600))
	target := filepath.Join(base, "directory") + `\..\directory`
	baseline := filepath.Join(base, "baseline")
	rtest.OK(t, os.Symlink(target, fixpath(baseline)))
	linkPath := filepath.Join(base, "link")
	rtest.OK(t, nodeCreateSymlinkAt(&data.Node{LinkTarget: target}, linkPath))
	expected, err := os.Readlink(baseline)
	rtest.OK(t, err)
	actual, err := os.Readlink(linkPath)
	rtest.OK(t, err)
	rtest.Equals(t, expected, actual)
	content, err := os.ReadFile(filepath.Join(linkPath, "file"))
	rtest.OK(t, err)
	rtest.Equals(t, "content", string(content))
}

func TestNodeCreateSymlinkWindowsTargetTypes(t *testing.T) {
	base := t.TempDir()
	rtest.OK(t, os.Mkdir(filepath.Join(base, "directory"), 0o700))
	rtest.OK(t, os.WriteFile(filepath.Join(base, "file"), []byte("content"), 0o600))
	for _, name := range []string{"directory", "file", "missing"} {
		t.Run(name, func(t *testing.T) {
			linkPath := filepath.Join(base, name+"-link")
			rtest.OK(t, nodeCreateSymlinkAt(&data.Node{LinkTarget: name}, linkPath))
			namep, err := windows.UTF16PtrFromString(fixpath(linkPath))
			rtest.OK(t, err)
			attrs, err := windows.GetFileAttributes(namep)
			rtest.OK(t, err)
			rtest.Assert(t, attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0, "expected a symlink")
			rtest.Equals(t, name == "directory", attrs&windows.FILE_ATTRIBUTE_DIRECTORY != 0)
			actual, err := os.Readlink(fixpath(linkPath))
			rtest.OK(t, err)
			rtest.Equals(t, name, actual)
		})
	}
}
