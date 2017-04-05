// +build !windows,!darwin

package restic_test

import (
	"os"
	"restic"
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
