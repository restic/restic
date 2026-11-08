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
