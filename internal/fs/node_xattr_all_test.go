//go:build darwin || freebsd || netbsd || linux || solaris || windows
// +build darwin freebsd netbsd linux solaris windows

package fs

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/restic/restic/internal/filter"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func setAndVerifyXattr(t *testing.T, file string, attrs []restic.ExtendedAttribute) {
	if runtime.GOOS == "windows" {
		// windows seems to convert the xattr name to upper case
		for i := range attrs {
			attrs[i].Name = strings.ToUpper(attrs[i].Name)
		}
	}

	node := &restic.Node{
		Type:               restic.NodeTypeFile,
		ExtendedAttributes: attrs,
	}
	/* restore all xattrs */
	rtest.OK(t, nodeRestoreExtendedAttributes(node, file, func(_ string) bool { return true }))

	nodeActual := &restic.Node{
		Type: restic.NodeTypeFile,
	}
	rtest.OK(t, nodeFillExtendedAttributes(nodeActual, file, false))

	rtest.Assert(t, nodeActual.Equals(*node), "xattr mismatch got %v expected %v", nodeActual.ExtendedAttributes, node.ExtendedAttributes)
}

func setAndVerifyXattrWithSelectFilter(t *testing.T, file string, testAttr []testXattrToRestore, xattrSelectFilter func(_ string) bool) {
	attrs := make([]restic.ExtendedAttribute, len(testAttr))
	for i := range testAttr {
		// windows seems to convert the xattr name to upper case
		if runtime.GOOS == "windows" {
			testAttr[i].xattr.Name = strings.ToUpper(testAttr[i].xattr.Name)
		}
		attrs[i] = testAttr[i].xattr
	}

	node := &restic.Node{
		Type:               restic.NodeTypeFile,
		ExtendedAttributes: attrs,
	}

	rtest.OK(t, nodeRestoreExtendedAttributes(node, file, xattrSelectFilter))

	nodeActual := &restic.Node{
		Type: restic.NodeTypeFile,
	}
	rtest.OK(t, nodeFillExtendedAttributes(nodeActual, file, false))

	// Check nodeActual to make sure only xattrs we expect are there
	for _, testAttr := range testAttr {
		xattrFound := false
		xattrRestored := false
		for _, restoredAttr := range nodeActual.ExtendedAttributes {
			if restoredAttr.Name == testAttr.xattr.Name {
				xattrFound = true
				xattrRestored = bytes.Equal(restoredAttr.Value, testAttr.xattr.Value)
				break
			}
		}
		if testAttr.shouldRestore {
			rtest.Assert(t, xattrFound, "xattr %s not restored", testAttr.xattr.Name)
			rtest.Assert(t, xattrRestored, "xattr %v value not restored", testAttr.xattr)
		} else {
			rtest.Assert(t, !xattrFound, "xattr %v should not have been restored", testAttr.xattr)
		}
	}
}

type testXattrToRestore struct {
	xattr         restic.ExtendedAttribute
	shouldRestore bool
}

func TestOverwriteXattr(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	rtest.OK(t, os.WriteFile(file, []byte("hello world"), 0o600))

	setAndVerifyXattr(t, file, []restic.ExtendedAttribute{
		{
			Name:  "user.foo",
			Value: []byte("bar"),
		},
	})

	setAndVerifyXattr(t, file, []restic.ExtendedAttribute{
		{
			Name:  "user.other",
			Value: []byte("some"),
		},
	})
}

func uppercaseOnWindows(patterns []string) []string {
	// windows seems to convert the xattr name to upper case
	if runtime.GOOS == "windows" {
		out := []string{}
		for _, pattern := range patterns {
			out = append(out, strings.ToUpper(pattern))
		}
		return out
	}
	return patterns
}

func TestOverwriteXattrWithSelectFilter(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file2")
	rtest.OK(t, os.WriteFile(file, []byte("hello world"), 0o600))

	noopWarnf := func(_ string, _ ...interface{}) {}

	// Set a filter as if the user passed in --include-xattr user.*
	xattrSelectFilter1 := func(xattrName string) bool {
		shouldInclude, _ := filter.IncludeByPattern(uppercaseOnWindows([]string{"user.*"}), noopWarnf)(xattrName)
		return shouldInclude
	}

	setAndVerifyXattrWithSelectFilter(t, file, []testXattrToRestore{
		{
			xattr: restic.ExtendedAttribute{
				Name:  "user.foo",
				Value: []byte("bar"),
			},
			shouldRestore: true,
		},
		{
			xattr: restic.ExtendedAttribute{
				Name:  "user.test",
				Value: []byte("testxattr"),
			},
			shouldRestore: true,
		},
		{
			xattr: restic.ExtendedAttribute{
				Name:  "security.other",
				Value: []byte("testing"),
			},
			shouldRestore: false,
		},
	}, xattrSelectFilter1)

	// Set a filter as if the user passed in --include-xattr user.*
	xattrSelectFilter2 := func(xattrName string) bool {
		shouldInclude, _ := filter.IncludeByPattern(uppercaseOnWindows([]string{"user.o*", "user.comm*"}), noopWarnf)(xattrName)
		return shouldInclude
	}

	setAndVerifyXattrWithSelectFilter(t, file, []testXattrToRestore{
		{
			xattr: restic.ExtendedAttribute{
				Name:  "user.other",
				Value: []byte("some"),
			},
			shouldRestore: true,
		},
		{
			xattr: restic.ExtendedAttribute{
				Name:  "security.other",
				Value: []byte("testing"),
			},
			shouldRestore: false,
		},
		{
			xattr: restic.ExtendedAttribute{
				Name:  "user.open",
				Value: []byte("door"),
			},
			shouldRestore: true,
		},
		{
			xattr: restic.ExtendedAttribute{
				Name:  "user.common",
				Value: []byte("testing"),
			},
			shouldRestore: true,
		},
		{
			xattr: restic.ExtendedAttribute{
				Name:  "user.bad",
				Value: []byte("dontincludeme"),
			},
			shouldRestore: false,
		},
	}, xattrSelectFilter2)
}
