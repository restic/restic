//go:build !windows
// +build !windows

package fs

import (
	"os"

	"github.com/restic/restic/internal/restic"
)

func lchown(name string, userName, groupName string) error {
	uid := lookupUid(userName)
	gid := lookupGid(groupName)

	return os.Lchown(name, int(uid), int(gid))
}

// nodeRestoreGenericAttributes is no-op.
func nodeRestoreGenericAttributes(node *restic.Node, _ string, warn func(msg string)) error {
	return restic.HandleAllUnknownGenericAttributesFound(node.GenericAttributes, warn)
}

// nodeFillGenericAttributes is a no-op.
func nodeFillGenericAttributes(_ *restic.Node, _ string, _ *ExtendedFileInfo) error {
	return nil
}
