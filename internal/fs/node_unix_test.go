//go:build !windows
// +build !windows

package fs

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/errors"
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

func checkFile(t testing.TB, fi fs.FileInfo, node *data.Node) {
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

	if node.Size != uint64(stat.Size) && node.Type != data.NodeTypeSymlink {
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

func checkDevice(t testing.TB, fi fs.FileInfo, node *data.Node) {
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

			fs := &Local{}
			meta, err := fs.OpenFile(test.filename, O_NOFOLLOW, true)
			rtest.OK(t, err)
			node, err := meta.ToNode(false, t.Logf)
			rtest.OK(t, err)
			rtest.OK(t, meta.Close())

			rtest.OK(t, err)

			switch node.Type {
			case data.NodeTypeFile, data.NodeTypeSymlink:
				checkFile(t, fi, node)
			case data.NodeTypeDev, data.NodeTypeCharDev:
				checkFile(t, fi, node)
				checkDevice(t, fi, node)
			default:
				t.Fatalf("invalid node type %q", node.Type)
			}
		})
	}
}

func TestMknodError(t *testing.T) {
	d := t.TempDir()
	// Call mkfifo, which calls mknod, as mknod may give
	// "operation not permitted" on Mac.
	err := mkfifo(d, 0)
	rtest.Assert(t, errors.Is(err, os.ErrExist), "want ErrExist, got %q", err)
	rtest.Assert(t, strings.Contains(err.Error(), d), "filename not in %q", err)
}
