//go:build darwin || freebsd || netbsd || linux || solaris

package fs

import (
	"os"
	"strings"
	"testing"

	"github.com/pkg/xattr"
	"github.com/restic/restic/internal/data"
	rtest "github.com/restic/restic/internal/test"
)

func TestIsListxattrPermissionError(t *testing.T) {
	xerr := &xattr.Error{
		Op:   "xattr.list",
		Name: "test",
		Err:  os.ErrPermission,
	}
	err := handleXattrErr(xerr)
	rtest.Assert(t, err != nil, "missing error")
	rtest.Assert(t, isListxattrPermissionError(err), "expected IsListxattrPermissionError to return true for %v", err)

	xerr.Err = os.ErrNotExist
	err = handleXattrErr(xerr)
	rtest.Assert(t, err != nil, "missing error")
	rtest.Assert(t, !isListxattrPermissionError(err), "expected IsListxattrPermissionError to return false for %v", err)
}

func TestNodeXattrsNeedRestore(t *testing.T) {
	dir := t.TempDir()

	// the filesystem may attach its own attributes (e.g. security.selinux),
	// so the tests only exercise user.* attributes
	userOnly := func(name string) bool { return strings.HasPrefix(name, "user.") }

	fi, err := os.Lstat(dir)
	rtest.OK(t, err)
	ex := ExtendedStat(fi)

	err = xattr.LSet(dir, "user.foo", []byte("bar"))
	if err != nil {
		t.Skipf("filesystem does not support xattrs: %v", err)
	}

	matching := &data.Node{
		Type: data.NodeTypeDir,
		ExtendedAttributes: []data.ExtendedAttribute{
			{Name: "user.foo", Value: []byte("bar")},
		},
	}
	differs, _ := nodeXattrsNeedRestore(matching, dir, ex, userOnly)
	rtest.Assert(t, !differs, "matching xattrs must not differ")

	// different value
	changed := *matching
	changed.ExtendedAttributes = []data.ExtendedAttribute{{Name: "user.foo", Value: []byte("baz")}}
	differs, _ = nodeXattrsNeedRestore(&changed, dir, ex, userOnly)
	rtest.Assert(t, differs, "different xattr value must differ")

	// missing attribute
	changed = *matching
	changed.ExtendedAttributes = []data.ExtendedAttribute{{Name: "user.other", Value: []byte("bar")}}
	differs, _ = nodeXattrsNeedRestore(&changed, dir, ex, userOnly)
	rtest.Assert(t, differs, "missing xattr must differ")

	// extra attribute on disk which would be removed
	changed = *matching
	changed.ExtendedAttributes = nil
	differs, _ = nodeXattrsNeedRestore(&changed, dir, ex, userOnly)
	rtest.Assert(t, differs, "extra xattr must differ")
}

// TestCanModifyXattrs pins the permission rules used to decide whether
// extended attribute differences can be restored: ownership always grants
// modification, a sticky directory requires ownership, and everything else
// only needs write access.
func TestCanModifyXattrs(t *testing.T) {
	dir := t.TempDir()

	// ownership grants modification regardless of the directory's permissions
	fi := &ExtendedFileInfo{UID: uint32(os.Geteuid()), Mode: os.ModeDir | 0000}
	rtest.Assert(t, canModifyXattrs(dir, fi),
		"owner must be able to modify xattrs")

	if os.Geteuid() == 0 {
		t.Log("running as root, permission probing is unreachable")
		return
	}

	// a sticky directory restricts xattr modifications to its owner even when
	// the current user has write access
	rtest.OK(t, os.Chmod(dir, os.ModeDir|os.ModeSticky|0770))
	fi = &ExtendedFileInfo{UID: 12345, Mode: os.ModeDir | os.ModeSticky | 0770}
	rtest.Assert(t, !canModifyXattrs(dir, fi),
		"non-owner must not be able to modify xattrs on a sticky directory")

	// write access is sufficient on a directory the snapshot says is owned
	// by someone else (Linux allows setting user.* attributes with write
	// permission; ownership is not required)
	rtest.OK(t, os.Chmod(dir, 0770))
	fi = &ExtendedFileInfo{UID: 12345, Mode: os.ModeDir | 0770}
	rtest.Assert(t, canModifyXattrs(dir, fi),
		"write access must allow xattr modification on a non-sticky directory")

	// without write access there is nothing to do
	rtest.OK(t, os.Chmod(dir, 0555))
	rtest.Assert(t, !canModifyXattrs(dir, fi),
		"xattrs cannot be modified without write access")
}

// TestNodeXattrsNeedRestoreAppliable pins the group-writable directory
// scenario: an xattr-only difference on a directory owned by another user
// is appliable for a process with write access, so the directory must not
// be reported as converged.
func TestNodeXattrsNeedRestoreAppliable(t *testing.T) {
	dir := t.TempDir()
	userOnly := func(name string) bool { return strings.HasPrefix(name, "user.") }

	err := xattr.LSet(dir, "user.foo", []byte("bar"))
	if err != nil {
		t.Skipf("filesystem does not support xattrs: %v", err)
	}
	if os.Geteuid() == 0 {
		t.Skip("requires an unprivileged caller")
	}
	changed := &data.Node{
		Type:               data.NodeTypeDir,
		ExtendedAttributes: []data.ExtendedAttribute{{Name: "user.foo", Value: []byte("baz")}},
	}

	rtest.OK(t, os.Chmod(dir, 0770))
	fi := &ExtendedFileInfo{UID: 12345, Mode: os.ModeDir | 0770}
	differs, appliable := nodeXattrsNeedRestore(changed, dir, fi, userOnly)
	rtest.Assert(t, differs, "different xattr value must differ")
	rtest.Assert(t, appliable,
		"xattr difference on a writable non-sticky directory must be appliable")
}
