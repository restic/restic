//go:build !windows
// +build !windows

package archiver

import (
	"os"
	"syscall"
	"testing"

	"github.com/restic/restic/internal/feature"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

type wrappedFileInfo struct {
	os.FileInfo
	sys  interface{}
	mode os.FileMode
}

func (fi wrappedFileInfo) Sys() interface{} {
	return fi.sys
}

func (fi wrappedFileInfo) Mode() os.FileMode {
	return fi.mode
}

// wrapFileInfo returns a new os.FileInfo with the mode, owner, and group fields changed.
func wrapFileInfo(fi os.FileInfo) os.FileInfo {
	// get the underlying stat_t and modify the values
	stat := fi.Sys().(*syscall.Stat_t)
	stat.Mode = mockFileInfoMode
	stat.Uid = mockFileInfoUID
	stat.Gid = mockFileInfoGID

	// wrap the os.FileInfo so we can return a modified stat_t
	res := wrappedFileInfo{
		FileInfo: fi,
		sys:      stat,
		mode:     mockFileInfoMode,
	}

	return res
}

func statAndSnapshot(t *testing.T, repo archiverRepo, name string) (*restic.Node, *restic.Node) {
	fi := lstat(t, name)
	want, err := restic.NodeFromFileInfo(name, fi, false)
	rtest.OK(t, err)

	_, node := snapshot(t, repo, fs.Local{}, nil, name)
	return want, node
}

func TestHardlinkMetadata(t *testing.T) {
	defer feature.TestSetFlag(t, feature.Flag, feature.DeviceIDForHardlinks, true)()

	files := TestDir{
		"testfile": TestFile{
			Content: "foo bar test file",
		},
		"linktarget": TestFile{
			Content: "test file",
		},
		"testlink": TestHardlink{
			Target: "./linktarget",
		},
		"testdir": TestDir{},
	}

	tempdir, repo := prepareTempdirRepoSrc(t, files)

	back := rtest.Chdir(t, tempdir)
	defer back()

	want, node := statAndSnapshot(t, repo, "testlink")
	rtest.Assert(t, node.DeviceID == want.DeviceID, "device id mismatch expected %v got %v", want.DeviceID, node.DeviceID)
	rtest.Assert(t, node.Links == want.Links, "link count mismatch expected %v got %v", want.Links, node.Links)
	rtest.Assert(t, node.Inode == want.Inode, "inode mismatch expected %v got %v", want.Inode, node.Inode)

	_, node = statAndSnapshot(t, repo, "testfile")
	rtest.Assert(t, node.DeviceID == 0, "device id mismatch for testfile expected %v got %v", 0, node.DeviceID)

	_, node = statAndSnapshot(t, repo, "testdir")
	rtest.Assert(t, node.DeviceID == 0, "device id mismatch for testdir expected %v got %v", 0, node.DeviceID)
}
