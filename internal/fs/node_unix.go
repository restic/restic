//go:build !windows
// +build !windows

package fs

import (
	"os"

	"github.com/restic/restic/internal/data"
)

func lchown(name string, uid, gid int) error {
	return os.Lchown(name, uid, gid)
}

// nodeRestoreGenericAttributes is no-op.
func nodeRestoreGenericAttributes(node *data.Node, _ string, warn func(msg string)) error {
	return data.HandleAllUnknownGenericAttributesFound(node.GenericAttributes, warn)
}

// nodeFillGenericAttributes is a no-op.
func nodeFillGenericAttributes(_ *data.Node, _ string, _ *ExtendedFileInfo) error {
	return nil
}
