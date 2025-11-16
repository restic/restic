//go:build !windows
// +build !windows

package fs

import (
	"os"

	"github.com/restic/restic/internal/data"
)

func lchown(name string, node *data.Node, lookupByName bool) error {
	var uid, gid uint32
	if lookupByName {
		uid = lookupUid(node.User)
		gid = lookupGid(node.Group)
	} else {
		uid = node.UID
		gid = node.GID
	}

	return os.Lchown(name, int(uid), int(gid))
}

// nodeRestoreGenericAttributes is no-op.
func nodeRestoreGenericAttributes(node *data.Node, _ string, warn func(msg string)) error {
	return data.HandleAllUnknownGenericAttributesFound(node.GenericAttributes, warn)
}

// nodeFillGenericAttributes is a no-op.
func nodeFillGenericAttributes(_ *data.Node, _ string, _ *ExtendedFileInfo) error {
	return nil
}
