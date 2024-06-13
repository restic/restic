//go:build darwin || freebsd || linux || solaris || windows
// +build darwin freebsd linux solaris windows

package restic

import (
	"os"
	"path/filepath"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func setAndVerifyXattr(t *testing.T, file string, attrs []ExtendedAttribute) {
	node := Node{
		ExtendedAttributes: attrs,
	}
	rtest.OK(t, node.restoreExtendedAttributes(file))

	nodeActual := Node{}
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
