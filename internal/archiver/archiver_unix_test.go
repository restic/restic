//go:build !windows
// +build !windows

package archiver

import (
	"testing"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/feature"
	"github.com/restic/restic/internal/fs"
	rtest "github.com/restic/restic/internal/test"
)

func statAndSnapshot(t *testing.T, repo archiverRepo, name string) (*data.Node, *data.Node) {
	want := nodeFromFile(t, &fs.Local{}, name)
	_, node := snapshot(t, repo, &fs.Local{}, nil, name)
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
