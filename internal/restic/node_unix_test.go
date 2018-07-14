// +build !windows

package restic

import (
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"
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

func checkFile(t testing.TB, stat *syscall.Stat_t, node *Node) {
	if uint32(node.Mode.Perm()) != uint32(stat.Mode&0777) {
		t.Errorf("Mode does not match, want %v, got %v", stat.Mode&0777, node.Mode)
	}

	if node.Inode != uint64(stat.Ino) {
		t.Errorf("Inode does not match, want %v, got %v", stat.Ino, node.Inode)
	}

	if node.DeviceID != uint64(stat.Dev) {
		t.Errorf("Dev does not match, want %v, got %v", stat.Dev, node.DeviceID)
	}

	if node.Size != uint64(stat.Size) {
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
	s, ok := toStatT(stat)
	if !ok {
		return
	}

	mtime := s.mtim()
	if node.ModTime != time.Unix(mtime.Unix()) {
		t.Errorf("ModTime does not match, want %v, got %v", time.Unix(mtime.Unix()), node.ModTime)
	}

	ctime := s.ctim()
	if node.ChangeTime != time.Unix(ctime.Unix()) {
		t.Errorf("ChangeTime does not match, want %v, got %v", time.Unix(ctime.Unix()), node.ChangeTime)
	}

	atime := s.atim()
	if node.AccessTime != time.Unix(atime.Unix()) {
		t.Errorf("AccessTime does not match, want %v, got %v", time.Unix(atime.Unix()), node.AccessTime)
	}

}

func checkDevice(t testing.TB, stat *syscall.Stat_t, node *Node) {
	if node.Device != uint64(stat.Rdev) {
		t.Errorf("Rdev does not match, want %v, got %v", stat.Rdev, node.Device)
	}
}

func TestNodeFromFileInfo(t *testing.T) {
	type Test struct {
		filename string
		canSkip  bool
	}
	var tests = []Test{
		{"node_test.go", false},
		{"/dev/sda", true},
	}

	// on darwin, users are not permitted to list the extended attributes of
	// /dev/null, therefore skip it.
	if runtime.GOOS != "darwin" {
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

			s, ok := fi.Sys().(*syscall.Stat_t)
			if !ok {
				t.Skipf("fi type is %T, not stat_t", fi.Sys())
				return
			}

			node, err := NodeFromFileInfo(test.filename, fi)
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
