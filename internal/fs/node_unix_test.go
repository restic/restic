//go:build !windows
// +build !windows

package fs

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

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

func checkFile(t testing.TB, fi fs.FileInfo, node *restic.Node) {
	t.Helper()

	stat := fi.Sys().(*syscall.Stat_t)

	if uint32(node.Mode.Perm()) != uint32(stat.Mode&0777) {
		t.Errorf("Mode does not match, want %v, got %v", stat.Mode&0777, node.Mode)
	}

	if node.Inode != uint64(stat.Ino) {
		t.Errorf("Inode does not match, want %v, got %v", stat.Ino, node.Inode)
	}

	if node.DeviceID != uint64(stat.Dev) {
		t.Errorf("Dev does not match, want %v, got %v", stat.Dev, node.DeviceID)
	}

	if node.Size != uint64(stat.Size) && node.Type != restic.NodeTypeSymlink {
		t.Errorf("Size does not match, want %v, got %v", stat.Size, node.Size)
	}

	if node.Links != uint64(stat.Nlink) {
		t.Errorf("Links does not match, want %v, got %v", stat.Nlink, node.Links)
	}

	if node.UID != stat.Uid {
		t.Errorf("UID does not match, want %v, got %v", stat.Uid, node.UID)
	}

	if node.GID != stat.Gid {
		t.Errorf("UID does not match, want %v, got %v", stat.Gid, node.GID)
	}

	// use the os dependent function to compare the timestamps
	s := ExtendedStat(fi)
	if node.ModTime != s.ModTime {
		t.Errorf("ModTime does not match, want %v, got %v", s.ModTime, node.ModTime)
	}
	if node.ChangeTime != s.ChangeTime {
		t.Errorf("ChangeTime does not match, want %v, got %v", s.ChangeTime, node.ChangeTime)
	}
	if node.AccessTime != s.AccessTime {
		t.Errorf("AccessTime does not match, want %v, got %v", s.AccessTime, node.AccessTime)
	}
}

func checkDevice(t testing.TB, fi fs.FileInfo, node *restic.Node) {
	stat := fi.Sys().(*syscall.Stat_t)
	if node.Device != uint64(stat.Rdev) {
		t.Errorf("Rdev does not match, want %v, got %v", stat.Rdev, node.Device)
	}
}

func TestNodeFromFileInfo(t *testing.T) {
	tmp := t.TempDir()
	symlink := filepath.Join(tmp, "symlink")
	rtest.OK(t, os.Symlink("target", symlink))

	type Test struct {
		filename string
		canSkip  bool
	}
	var tests = []Test{
		{"node_test.go", false},
		{"/dev/sda", true},
		{symlink, false},
	}

	// on darwin, users are not permitted to list the extended attributes of
	// /dev/null, therefore skip it.
	// on solaris, /dev/null is a symlink to a device node in /devices
	// which does not support extended attributes, therefore skip it.
	if runtime.GOOS != "darwin" && runtime.GOOS != "solaris" {
		tests = append(tests, Test{"/dev/null", true})
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			fi, found := stat(t, test.filename)
			if !found && test.canSkip {
				t.Skipf("%v not found in filesystem", test.filename)
				return
			}

			if fi.Sys() == nil {
				t.Skip("fi.Sys() is nil")
				return
			}

			node, err := NodeFromFileInfo(test.filename, fi, false)
			if err != nil {
				t.Fatal(err)
			}

			switch node.Type {
			case restic.NodeTypeFile, restic.NodeTypeSymlink:
				checkFile(t, fi, node)
			case restic.NodeTypeDev, restic.NodeTypeCharDev:
				checkFile(t, fi, node)
				checkDevice(t, fi, node)
			default:
				t.Fatalf("invalid node type %q", node.Type)
			}
		})
	}
}
