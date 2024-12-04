//go:build !windows
// +build !windows

package fs

import (
	"os"

	"github.com/restic/restic/internal/restic"
)

func lchown(name string, uid, gid int) error {
	return os.Lchown(name, uid, gid)
}

// nodeRestoreGenericAttributes is no-op.
func nodeRestoreGenericAttributes(node *restic.Node, _ string, warn func(msg string)) error {
	return restic.HandleAllUnknownGenericAttributesFound(node.GenericAttributes, warn)
}

// nodeFillGenericAttributes is a no-op.
func nodeFillGenericAttributes(_ *restic.Node, _ string, _ *ExtendedFileInfo) error {
	return nil
}
