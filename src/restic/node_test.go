package restic_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"restic"
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
		Type:       "file",
		Content:    restic.IDs{},
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0604,
		ModTime:    parseTime("2015-05-14 21:07:23.111"),
		AccessTime: parseTime("2015-05-14 21:07:24.222"),
		ChangeTime: parseTime("2015-05-14 21:07:25.333"),
	},
	restic.Node{
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
	restic.Node{
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
	restic.Node{
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
	restic.Node{
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
	restic.Node{
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
	restic.Node{
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
	restic.Node{
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
	tempdir, err := ioutil.TempDir(TestTempDir, "restic-test-")
	OK(t, err)

	defer func() {
		if TestCleanupTempDirs {
			RemoveAll(t, tempdir)
		} else {
			t.Logf("leaving tempdir at %v", tempdir)
		}
	}()

	idx := restic.NewHardlinkIndex()

	for _, test := range nodeTests {
		nodePath := filepath.Join(tempdir, test.Name)
		OK(t, test.CreateAt(nodePath, nil, idx))

		if test.Type == "symlink" && runtime.GOOS == "windows" {
			continue
		}
		if test.Type == "dir" {
			OK(t, test.RestoreTimestamps(nodePath))
		}

		fi, err := os.Lstat(nodePath)
		OK(t, err)

		n2, err := restic.NodeFromFileInfo(nodePath, fi)
		OK(t, err)

		Assert(t, test.Name == n2.Name,
			"%v: name doesn't match (%v != %v)", test.Type, test.Name, n2.Name)
		Assert(t, test.Type == n2.Type,
			"%v: type doesn't match (%v != %v)", test.Type, test.Type, n2.Type)
		Assert(t, test.Size == n2.Size,
			"%v: size doesn't match (%v != %v)", test.Size, test.Size, n2.Size)

		if runtime.GOOS != "windows" {
			Assert(t, test.UID == n2.UID,
				"%v: UID doesn't match (%v != %v)", test.Type, test.UID, n2.UID)
			Assert(t, test.GID == n2.GID,
				"%v: GID doesn't match (%v != %v)", test.Type, test.GID, n2.GID)
			if test.Type != "symlink" {
				Assert(t, test.Mode == n2.Mode,
					"%v: mode doesn't match (0%o != 0%o)", test.Type, test.Mode, n2.Mode)
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

func stat(t testing.TB, filename string) (fi os.FileInfo, ok bool) {
	fi, err := os.Lstat(filename)
	if err != nil && os.IsNotExist(err) {
		return fi, false
	}

	if err != nil {
		t.Fatal(err)
	}

	return fi, true
}

func checkFile(t testing.TB, stat *syscall.Stat_t, node *restic.Node) {
	if uint32(node.Mode.Perm()) != stat.Mode&0777 {
		t.Errorf("Mode does not match, want %v, got %v", stat.Mode&0777, node.Mode)
	}

	if node.Inode != stat.Ino {
		t.Errorf("Inode does not match, want %v, got %v", stat.Ino, node.Inode)
	}

	if node.DeviceID != stat.Dev {
		t.Errorf("Dev does not match, want %v, got %v", stat.Dev, node.DeviceID)
	}

	if node.Size != uint64(stat.Size) {
		t.Errorf("Size does not match, want %v, got %v", stat.Size, node.Size)
	}

	if node.Links != stat.Nlink {
		t.Errorf("Links does not match, want %v, got %v", stat.Nlink, node.Links)
	}

	if node.ModTime != time.Unix(stat.Mtim.Unix()) {
		t.Errorf("ModTime does not match, want %v, got %v", time.Unix(stat.Mtim.Unix()), node.ModTime)
	}

	if node.ChangeTime != time.Unix(stat.Ctim.Unix()) {
		t.Errorf("ChangeTime does not match, want %v, got %v", time.Unix(stat.Ctim.Unix()), node.ChangeTime)
	}

	if node.AccessTime != time.Unix(stat.Atim.Unix()) {
		t.Errorf("AccessTime does not match, want %v, got %v", time.Unix(stat.Atim.Unix()), node.AccessTime)
	}

	if node.UID != stat.Uid {
		t.Errorf("UID does not match, want %v, got %v", stat.Uid, node.UID)
	}

	if node.GID != stat.Gid {
		t.Errorf("UID does not match, want %v, got %v", stat.Gid, node.GID)
	}
}

func checkDevice(t testing.TB, stat *syscall.Stat_t, node *restic.Node) {
	if node.Device != stat.Rdev {
		t.Errorf("Rdev does not match, want %v, got %v", stat.Rdev, node.Device)
	}
}

func TestNodeFromFileInfo(t *testing.T) {
	var tests = []struct {
		filename string
		canSkip  bool
	}{
		{"node_test.go", false},
		{"/dev/null", true},
		{"/dev/sda", true},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			fi, found := stat(t, test.filename)
			if !found && test.canSkip {
				t.Skipf("%v not found in filesystem")
				return
			}

			if fi.Sys() == nil {
				t.Skip("fi.Sys() is nil")
				return
			}

			s, ok := fi.Sys().(*syscall.Stat_t)
			if !ok {
				t.Skip("fi type is %T, not stat_t", fi.Sys())
				return
			}

			node, err := restic.NodeFromFileInfo(test.filename, fi)
			if err != nil {
				t.Fatal(err)
			}

			switch node.Type {
			case "file":
				checkFile(t, s, node)
			case "dev", "chardev":
				checkFile(t, s, node)
				checkDevice(t, s, node)
			default:
				t.Fatalf("invalid node type %q", node.Type)
			}
		})
	}
}
