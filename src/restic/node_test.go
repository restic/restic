package restic_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"restic"
	"restic/backend"
	. "restic/test"
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

	OK(t, tempfile.Close())
	RemoveAll(t, tempfile.Name())
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

	OK(t, tempfile.Close())
	RemoveAll(t, tempfile.Name())
}

func parseTime(s string) time.Time {
	t, err := time.Parse("2006-01-02 15:04:05.999", s)
	if err != nil {
		panic(err)
	}

	return t.Local()
}

var nodeTests = []restic.Node{
	restic.Node{
		Name:       "testFile",
		FileType:   "file",
		Content:    []backend.ID{},
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0604,
		ModTime:    parseTime("2015-05-14 21:07:23.111"),
		AccessTime: parseTime("2015-05-14 21:07:24.222"),
		ChangeTime: parseTime("2015-05-14 21:07:25.333"),
	},
	restic.Node{
		Name:       "testSuidFile",
		FileType:   "file",
		Content:    []backend.ID{},
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0755 | os.ModeSetuid,
		ModTime:    parseTime("2015-05-14 21:07:23.111"),
		AccessTime: parseTime("2015-05-14 21:07:24.222"),
		ChangeTime: parseTime("2015-05-14 21:07:25.333"),
	},
	restic.Node{
		Name:       "testSuidFile2",
		FileType:   "file",
		Content:    []backend.ID{},
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0755 | os.ModeSetgid,
		ModTime:    parseTime("2015-05-14 21:07:23.111"),
		AccessTime: parseTime("2015-05-14 21:07:24.222"),
		ChangeTime: parseTime("2015-05-14 21:07:25.333"),
	},
	restic.Node{
		Name:       "testSticky",
		FileType:   "file",
		Content:    []backend.ID{},
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0755 | os.ModeSticky,
		ModTime:    parseTime("2015-05-14 21:07:23.111"),
		AccessTime: parseTime("2015-05-14 21:07:24.222"),
		ChangeTime: parseTime("2015-05-14 21:07:25.333"),
	},
	restic.Node{
		Name:       "testDir",
		FileType:   "dir",
		Subtree:    nil,
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0750 | os.ModeDir,
		ModTime:    parseTime("2015-05-14 21:07:23.111"),
		AccessTime: parseTime("2015-05-14 21:07:24.222"),
		ChangeTime: parseTime("2015-05-14 21:07:25.333"),
	},
	restic.Node{
		Name:       "testSymlink",
		FileType:   "symlink",
		LinkTarget: "invalid",
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0777 | os.ModeSymlink,
		ModTime:    parseTime("2015-05-14 21:07:23.111"),
		AccessTime: parseTime("2015-05-14 21:07:24.222"),
		ChangeTime: parseTime("2015-05-14 21:07:25.333"),
	},
}

func TestNodeRestoreAt(t *testing.T) {
	tempdir, err := ioutil.TempDir(TestTempDir, "restic-test-")
	OK(t, err)

	defer func() {
		if TestCleanupTempDirs {
			RemoveAll(t, tempdir)
		} else {
			t.Logf("leaving tempdir at %v", tempdir)
		}
	}()

	for _, test := range nodeTests {
		nodePath := filepath.Join(tempdir, test.Name)
		OK(t, test.CreateAt(nodePath, nil))

		if test.FileType == "symlink" && runtime.GOOS == "windows" {
			continue
		}
		if test.FileType == "dir" {
			OK(t, test.RestoreTimestamps(nodePath))
		}

		fi, err := os.Lstat(nodePath)
		OK(t, err)

		n2, err := restic.NodeFromFileInfo(nodePath, fi)
		OK(t, err)

		Assert(t, test.Name == n2.Name,
			"%v: name doesn't match (%v != %v)", test.FileType, test.Name, n2.Name)
		Assert(t, test.FileType == n2.FileType,
			"%v: type doesn't match (%v != %v)", test.FileType, test.FileType, n2.FileType)
		Assert(t, test.Size == n2.Size,
			"%v: size doesn't match (%v != %v)", test.Size, test.Size, n2.Size)

		if runtime.GOOS != "windows" {
			Assert(t, test.UID == n2.UID,
				"%v: UID doesn't match (%v != %v)", test.FileType, test.UID, n2.UID)
			Assert(t, test.GID == n2.GID,
				"%v: GID doesn't match (%v != %v)", test.FileType, test.GID, n2.GID)
			if test.FileType != "symlink" {
				Assert(t, test.Mode == n2.Mode,
					"%v: mode doesn't match (0%o != 0%o)", test.FileType, test.Mode, n2.Mode)
			}
		}

		AssertFsTimeEqual(t, "AccessTime", test.FileType, test.AccessTime, n2.AccessTime)
		AssertFsTimeEqual(t, "ModTime", test.FileType, test.ModTime, n2.ModTime)
	}
}

func AssertFsTimeEqual(t *testing.T, label string, nodeType string, t1 time.Time, t2 time.Time) {
	var equal bool

	// Go currently doesn't support setting timestamps of symbolic links on darwin and bsd
	if nodeType == "symlink" {
		switch runtime.GOOS {
		case "darwin", "freebsd", "openbsd":
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

	Assert(t, equal, "%s: %s doesn't match (%v != %v)", label, nodeType, t1, t2)
}
