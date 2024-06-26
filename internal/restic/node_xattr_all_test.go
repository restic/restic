//go:build darwin || freebsd || linux || solaris || windows
// +build darwin freebsd linux solaris windows

package restic

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func setAndVerifyXattr(t *testing.T, file string, attrs []ExtendedAttribute) {
	if runtime.GOOS == "windows" {
		// windows seems to convert the xattr name to upper case
		for i := range attrs {
			attrs[i].Name = strings.ToUpper(attrs[i].Name)
		}
	}

	node := Node{
		Type:               "file",
		ExtendedAttributes: attrs,
	}
	rtest.OK(t, node.restoreExtendedAttributes(file))

	nodeActual := Node{
		Type: "file",
	}
	rtest.OK(t, nodeActual.fillExtendedAttributes(file, false))

	rtest.Assert(t, nodeActual.sameExtendedAttributes(node), "xattr mismatch got %v expected %v", nodeActual.ExtendedAttributes, node.ExtendedAttributes)
}

func TestOverwriteXattr(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	rtest.OK(t, os.WriteFile(file, []byte("hello world"), 0o600))

	setAndVerifyXattr(t, file, []ExtendedAttribute{
		{
			Name:  "user.foo",
			Value: []byte("bar"),
		},
	})

	setAndVerifyXattr(t, file, []ExtendedAttribute{
		{
			Name:  "user.other",
			Value: []byte("some"),
		},
	})
}
