//go:build darwin || freebsd || linux || solaris || windows

package fs

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func TestXattrNoFollow(t *testing.T) {
	xattrs := []restic.ExtendedAttribute{
		{
			Name:  "user.foo",
			Value: []byte("bar"),
		},
	}
	if runtime.GOOS == "windows" {
		// windows seems to convert the xattr name to upper case
		for i := range xattrs {
			xattrs[i].Name = strings.ToUpper(xattrs[i].Name)
		}
	}

	setXattrs := func(path string) {
		node := &restic.Node{
			Type:               restic.NodeTypeFile,
			ExtendedAttributes: xattrs,
		}
		rtest.OK(t, nodeRestoreExtendedAttributes(node, path))
	}
	checkXattrs := func(expected []restic.ExtendedAttribute) func(t *testing.T, node *restic.Node) {
		return func(t *testing.T, node *restic.Node) {
			rtest.Equals(t, expected, node.ExtendedAttributes, "xattr mismatch for file")
		}
	}

	setupSymlinkTest := func(t *testing.T, path string) {
		rtest.OK(t, os.WriteFile(path+"file", []byte("example"), 0o600))
		setXattrs(path + "file")
		rtest.OK(t, os.Symlink(path+"file", path))
	}

	for _, test := range []fsLocalMetadataTestcase{
		{
			name: "file",
			setup: func(t *testing.T, path string) {
				rtest.OK(t, os.WriteFile(path, []byte("example"), 0o600))
				setXattrs(path)
			},
			nodeType: restic.NodeTypeFile,
			check:    checkXattrs(xattrs),
		},
		{
			name:     "symlink",
			setup:    setupSymlinkTest,
			nodeType: restic.NodeTypeSymlink,
			check:    checkXattrs([]restic.ExtendedAttribute{}),
		},
		{
			name:     "symlink file",
			follow:   true,
			setup:    setupSymlinkTest,
			nodeType: restic.NodeTypeFile,
			check:    checkXattrs(xattrs),
		},
	} {
		testHandleVariants(t, func(t *testing.T) {
			runFSLocalTestcase(t, test)
		})
	}
}
