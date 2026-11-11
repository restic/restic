//go:build !windows

package fs

import (
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

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

			fs := NewLocal()
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

func TestLchown(t *testing.T) {
	usr, err := user.Current()
	rtest.OK(t, err)

	uid, err := strconv.Atoi(usr.Uid)
	rtest.OK(t, err)

	gid, err := strconv.Atoi(usr.Gid)
	rtest.OK(t, err)

	d := t.TempDir()
	f := d + "/test.txt"
	err = os.WriteFile(f, []byte(""), 0o700)
	rtest.OK(t, err)

	t.Run("by UID/GID", func(t *testing.T) {
		n := &data.Node{
			UID: uint32(uid),
			GID: uint32(gid),
		}

		err = lchown(f, n, false)
		rtest.OK(t, err)
	})

	t.Run("by user name and group name", func(t *testing.T) {
		group, err := user.LookupGroupId(strconv.Itoa(gid))
		rtest.OK(t, err)

		n := &data.Node{
			User:  usr.Username,
			Group: group.Name,
		}

		err = lchown(f, n, true)
		rtest.OK(t, err)
	})
}

func TestNodeMetadataNeedsRestore(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "dir")
	rtest.OK(t, os.Mkdir(dir, 0755))
	// explicitly set mode, the umask may have restricted the permissions
	rtest.OK(t, os.Chmod(dir, 0755))
	modtime := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	rtest.OK(t, os.Chtimes(dir, modtime, modtime))

	var warnings []string
	warn := func(msg string) { warnings = append(warnings, msg) }
	// exclude all xattrs from the comparison: the test system may attach
	// its own attributes (e.g. security.selinux) which are covered by the
	// xattr-specific tests instead
	noXattrs := func(string) bool { return false }

	matching := &data.Node{
		Name:    "dir",
		Type:    data.NodeTypeDir,
		Mode:    0755 | os.ModeDir,
		ModTime: modtime,
		UID:     uint32(os.Getuid()),
		GID:     uint32(os.Getgid()),
	}

	rtest.Assert(t, !NodeMetadataNeedsRestore(matching, dir, warn, noXattrs, false),
		"matching dir must not need a restore")
	rtest.Equals(t, 0, len(warnings))

	// a node identical except for a zero timestamp (the serialization
	// default) must be detected as different
	changed := *matching
	changed.ModTime = time.Time{}
	rtest.Assert(t, NodeMetadataNeedsRestore(&changed, dir, warn, noXattrs, false),
		"different mtime must need a restore")
	rtest.Equals(t, 0, len(warnings))

	changed = *matching
	changed.Mode = 0700 | os.ModeDir
	rtest.Assert(t, NodeMetadataNeedsRestore(&changed, dir, warn, noXattrs, false),
		"different mode must need a restore")
	rtest.Equals(t, 0, len(warnings))

	// ownership that differs from the desired state
	changed = *matching
	changed.UID = 0
	changed.GID = 0
	needsRestore := NodeMetadataNeedsRestore(&changed, dir, warn, noXattrs, false)
	if os.Geteuid() == 0 {
		rtest.Assert(t, needsRestore, "root can change the owner, so a restore is needed")
		rtest.Equals(t, 0, len(warnings))
	} else {
		rtest.Assert(t, !needsRestore,
			"an unprivileged user cannot change the owner, so no restore is needed")
		rtest.Assert(t, len(warnings) > 0, "expected a warning for the un-appliable ownership change")
	}
	warnings = nil

	// a missing path can never be proven unchanged
	rtest.Assert(t, NodeMetadataNeedsRestore(matching, filepath.Join(dir, "nonexistent"), warn, noXattrs, false),
		"missing path must need a restore")
}
