//go:build darwin || freebsd || netbsd || linux || solaris || windows
// +build darwin freebsd netbsd linux solaris windows

package fs

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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
	rtest.OK(t, nodeRestoreExtendedAttributes(node, file))

	nodeActual := &restic.Node{
		Type: restic.NodeTypeFile,
	}
	rtest.OK(t, nodeFillExtendedAttributes(nodeActual, file, false))

	rtest.Assert(t, nodeActual.Equals(*node), "xattr mismatch got %v expected %v", nodeActual.ExtendedAttributes, node.ExtendedAttributes)
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
