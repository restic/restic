//go:build !windows

package fs

import (
	"fmt"
	"os"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/ui"
)

func lchown(name string, node *data.Node, lookupByName bool, warn func(msg string)) error {
	uid := node.UID
	gid := node.GID
	if lookupByName {
		// Fall back to the numeric ids stored in the snapshot if a name is
		// missing or cannot be resolved on this host.
		if node.User != "" {
			if id, ok := lookupUid(node.User); ok {
				uid = id
			} else if warn != nil {
				warn(fmt.Sprintf("cannot resolve user %q for %s, using uid %d", node.User, ui.Quote(name), uid))
			}
		}
		if node.Group != "" {
			if id, ok := lookupGid(node.Group); ok {
				gid = id
			} else if warn != nil {
				warn(fmt.Sprintf("cannot resolve group %q for %s, using gid %d", node.Group, ui.Quote(name), gid))
			}
		}
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
