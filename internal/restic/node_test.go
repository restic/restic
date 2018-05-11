package restic_test

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func BenchmarkNodeFillUser(t *testing.B) {
	tempfile, err := ioutil.TempFile("", "restic-test-temp-")
	if err != nil {
		t.Fatal(err)
	}

	fi, err := tempfile.Stat()
	if err != nil {
		t.Fatal(err)
	}

	path := tempfile.Name()

	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		restic.NodeFromFileInfo(path, fi)
	}

	rtest.OK(t, tempfile.Close())
	rtest.RemoveAll(t, tempfile.Name())
}

func BenchmarkNodeFromFileInfo(t *testing.B) {
	tempfile, err := ioutil.TempFile("", "restic-test-temp-")
	if err != nil {
		t.Fatal(err)
	}

	fi, err := tempfile.Stat()
	if err != nil {
		t.Fatal(err)
	}

	path := tempfile.Name()

	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		_, err := restic.NodeFromFileInfo(path, fi)
		if err != nil {
			t.Fatal(err)
		}
	}

	rtest.OK(t, tempfile.Close())
	rtest.RemoveAll(t, tempfile.Name())
}

func parseTime(s string) time.Time {
	t, err := time.Parse("2006-01-02 15:04:05.999", s)
	if err != nil {
		panic(err)
	}

	return t.Local()
}

var nodeTests = []restic.Node{
	{
		Name:       "testFile",
		Type:       "file",
		Content:    restic.IDs{},
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0604,
		ModTime:    parseTime("2015-05-14 21:07:23.111"),
		AccessTime: parseTime("2015-05-14 21:07:24.222"),
		ChangeTime: parseTime("2015-05-14 21:07:25.333"),
	},
	{
		Name:       "testSuidFile",
		Type:       "file",
		Content:    restic.IDs{},
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0755 | os.ModeSetuid,
		ModTime:    parseTime("2015-05-14 21:07:23.111"),
		AccessTime: parseTime("2015-05-14 21:07:24.222"),
		ChangeTime: parseTime("2015-05-14 21:07:25.333"),
	},
	{
		Name:       "testSuidFile2",
		Type:       "file",
		Content:    restic.IDs{},
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0755 | os.ModeSetgid,
		ModTime:    parseTime("2015-05-14 21:07:23.111"),
		AccessTime: parseTime("2015-05-14 21:07:24.222"),
		ChangeTime: parseTime("2015-05-14 21:07:25.333"),
	},
	{
		Name:       "testSticky",
		Type:       "file",
		Content:    restic.IDs{},
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0755 | os.ModeSticky,
		ModTime:    parseTime("2015-05-14 21:07:23.111"),
		AccessTime: parseTime("2015-05-14 21:07:24.222"),
		ChangeTime: parseTime("2015-05-14 21:07:25.333"),
	},
	{
		Name:       "testDir",
		Type:       "dir",
		Subtree:    nil,
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0750 | os.ModeDir,
		ModTime:    parseTime("2015-05-14 21:07:23.111"),
		AccessTime: parseTime("2015-05-14 21:07:24.222"),
		ChangeTime: parseTime("2015-05-14 21:07:25.333"),
	},
	{
		Name:       "testSymlink",
		Type:       "symlink",
		LinkTarget: "invalid",
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0777 | os.ModeSymlink,
		ModTime:    parseTime("2015-05-14 21:07:23.111"),
		AccessTime: parseTime("2015-05-14 21:07:24.222"),
		ChangeTime: parseTime("2015-05-14 21:07:25.333"),
	},

	// include "testFile" and "testDir" again with slightly different
	// metadata, so we can test if CreateAt works with pre-existing files.
	{
		Name:       "testFile",
		Type:       "file",
		Content:    restic.IDs{},
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0604,
		ModTime:    parseTime("2005-05-14 21:07:03.111"),
		AccessTime: parseTime("2005-05-14 21:07:04.222"),
		ChangeTime: parseTime("2005-05-14 21:07:05.333"),
	},
	{
		Name:       "testDir",
		Type:       "dir",
		Subtree:    nil,
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0750 | os.ModeDir,
		ModTime:    parseTime("2005-05-14 21:07:03.111"),
		AccessTime: parseTime("2005-05-14 21:07:04.222"),
		ChangeTime: parseTime("2005-05-14 21:07:05.333"),
	},
}

func TestNodeRestoreAt(t *testing.T) {
	tempdir, err := ioutil.TempDir(rtest.TestTempDir, "restic-test-")
	rtest.OK(t, err)

	defer func() {
		if rtest.TestCleanupTempDirs {
			rtest.RemoveAll(t, tempdir)
		} else {
			t.Logf("leaving tempdir at %v", tempdir)
		}
	}()

	for _, test := range nodeTests {
		nodePath := filepath.Join(tempdir, test.Name)
		rtest.OK(t, test.CreateAt(context.TODO(), nodePath, nil))
		rtest.OK(t, test.RestoreMetadata(nodePath))

		if test.Type == "symlink" && runtime.GOOS == "windows" {
			continue
		}
		if test.Type == "dir" {
			rtest.OK(t, test.RestoreTimestamps(nodePath))
		}

		fi, err := os.Lstat(nodePath)
		rtest.OK(t, err)

		n2, err := restic.NodeFromFileInfo(nodePath, fi)
		rtest.OK(t, err)

		rtest.Assert(t, test.Name == n2.Name,
			"%v: name doesn't match (%v != %v)", test.Type, test.Name, n2.Name)
		rtest.Assert(t, test.Type == n2.Type,
			"%v: type doesn't match (%v != %v)", test.Type, test.Type, n2.Type)
		rtest.Assert(t, test.Size == n2.Size,
			"%v: size doesn't match (%v != %v)", test.Size, test.Size, n2.Size)

		if runtime.GOOS != "windows" {
			rtest.Assert(t, test.UID == n2.UID,
				"%v: UID doesn't match (%v != %v)", test.Type, test.UID, n2.UID)
			rtest.Assert(t, test.GID == n2.GID,
				"%v: GID doesn't match (%v != %v)", test.Type, test.GID, n2.GID)
			if test.Type != "symlink" {
				// On OpenBSD only root can set sticky bit (see sticky(8)).
				if runtime.GOOS != "openbsd" && runtime.GOOS != "netbsd" && test.Name == "testSticky" {
					rtest.Assert(t, test.Mode == n2.Mode,
						"%v: mode doesn't match (0%o != 0%o)", test.Type, test.Mode, n2.Mode)
				}
			}
		}

		AssertFsTimeEqual(t, "AccessTime", test.Type, test.AccessTime, n2.AccessTime)
		AssertFsTimeEqual(t, "ModTime", test.Type, test.ModTime, n2.ModTime)
	}
}

func AssertFsTimeEqual(t *testing.T, label string, nodeType string, t1 time.Time, t2 time.Time) {
	var equal bool

	// Go currently doesn't support setting timestamps of symbolic links on darwin and bsd
	if nodeType == "symlink" {
		switch runtime.GOOS {
		case "darwin", "freebsd", "openbsd", "netbsd":
			return
		}
	}

	switch runtime.GOOS {
	case "darwin":
		// HFS+ timestamps don't support sub-second precision,
		// see https://en.wikipedia.org/wiki/Comparison_of_file_systems
		diff := int(t1.Sub(t2).Seconds())
		equal = diff == 0
	default:
		equal = t1.Equal(t2)
	}

	rtest.Assert(t, equal, "%s: %s doesn't match (%v != %v)", label, nodeType, t1, t2)
}
