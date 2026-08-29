//go:build !windows

package archiver

import (
	"path/filepath"
	"testing"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/feature"
	"github.com/restic/restic/internal/fs"
	rtest "github.com/restic/restic/internal/test"
	"golang.org/x/sys/unix"
)

func statAndSnapshot(t *testing.T, repo archiverRepo, name string) (*data.Node, *data.Node) {
	want := nodeFromFile(t, fs.NewLocal(), name)
	_, node := snapshot(t, repo, fs.NewLocal(), nil, name)
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
	// the device ID is replaced by a virtual one that stays the same across backup runs
	rtest.Assert(t, node.DeviceID != 0, "device id for hardlink must not be empty")
	rtest.Assert(t, node.DeviceID != want.DeviceID, "device id must not be the one reported by the filesystem, got %v", node.DeviceID)
	rtest.Assert(t, node.Links == want.Links, "link count mismatch expected %v got %v", want.Links, node.Links)
	rtest.Assert(t, node.Inode == want.Inode, "inode mismatch expected %v got %v", want.Inode, node.Inode)

	_, node = statAndSnapshot(t, repo, "testfile")
	rtest.Assert(t, node.DeviceID == 0, "device id mismatch for testfile expected %v got %v", 0, node.DeviceID)

	_, node = statAndSnapshot(t, repo, "testdir")
	rtest.Assert(t, node.DeviceID == 0, "device id mismatch for testdir expected %v got %v", 0, node.DeviceID)
}

func TestDeviceIDOnlyForHardlinkedNodes(t *testing.T) {
	defer feature.TestSetFlag(t, feature.Flag, feature.DeviceIDForHardlinks, true)()

	files := TestDir{
		"linktarget": TestFile{
			Content: "test file",
		},
		"testlink": TestHardlink{
			Target: "./linktarget",
		},
		"testsymlink": TestSymlink{
			Target: "./linktarget",
		},
	}

	tempdir, repo := prepareTempdirRepoSrc(t, files)

	back := rtest.Chdir(t, tempdir)
	defer back()

	// TestDir cannot create fifos
	rtest.OK(t, unix.Mkfifo("testfifo", 0o600))

	// a fifo stores no link count and must not be mistaken for a hardlinked node
	_, node := statAndSnapshot(t, repo, "testfifo")
	rtest.Equals(t, uint64(0), node.Links)
	rtest.Equals(t, uint64(0), node.DeviceID)

	// the same applies to a symlink without hardlinks
	_, node = statAndSnapshot(t, repo, "testsymlink")
	rtest.Equals(t, uint64(0), node.DeviceID)

	// a hardlinked file still stores one
	_, node = statAndSnapshot(t, repo, "testlink")
	rtest.Assert(t, node.Links > 1, "expected more than one link, got %v", node.Links)
	rtest.Assert(t, node.DeviceID != 0, "device ID must be stored for a hardlinked file")
}

func TestHardlinkVirtualDeviceID(t *testing.T) {
	defer feature.TestSetFlag(t, feature.Flag, feature.DeviceIDForHardlinks, true)()

	files := TestDir{
		"testdir": TestDir{
			"linktarget": TestFile{
				Content: "test file",
			},
			"testfile": TestFile{
				Content: "foo bar test file",
			},
			"testlink": TestHardlink{
				Target: "./linktarget",
			},
		},
	}

	tempdir, repo := prepareTempdirRepoSrc(t, files)

	back := rtest.Chdir(t, tempdir)
	defer back()

	want := nodeFromFile(t, fs.NewLocal(), filepath.Join("testdir", "linktarget"))

	_, dir := snapshot(t, repo, fs.NewLocal(), nil, "testdir")
	nodes := loadTreeNodes(t, repo, *dir.Subtree)

	// Hardlinked files must share a device ID within a snapshot. The restorer
	// uses it to recreate the hardlinks.
	rtest.Assert(t, nodes["linktarget"].DeviceID != 0, "device ID must be stored for a hardlinked file")
	rtest.Assert(t, nodes["linktarget"].DeviceID != want.DeviceID, "device ID must not be the one reported by the filesystem")
	rtest.Equals(t, nodes["linktarget"].DeviceID, nodes["testlink"].DeviceID)
	rtest.Equals(t, nodes["linktarget"].Inode, nodes["testlink"].Inode)

	// a file without hardlinks stores no device ID
	rtest.Equals(t, uint64(0), nodes["testfile"].DeviceID)
}
