//go:build windows

package fs

import (
	"github.com/restic/restic/internal/data"
)

// Windows does not restore unix-style ownership (lchown is a no-op), so there
// is never an ownership difference to restore.
func nodeOwnershipNeedsRestore(_ *data.Node, _ *ExtendedFileInfo, _ bool) (differs, appliable bool) {
	return false, false
}

func canModifyMetadata(_ *ExtendedFileInfo) bool {
	return true
}

// nodeXattrsNeedRestore stays conservative: comparing the raw byte streams of
// windows extended attributes is out of scope, so a node carrying any
// selected attribute counts as needing a restore.
func nodeXattrsNeedRestore(node *data.Node, _ string, _ *ExtendedFileInfo, xattrSelectFilter func(xattrName string) bool) (differs, appliable bool) {
	for _, attr := range node.ExtendedAttributes {
		if xattrSelectFilter(attr.Name) {
			return true, true
		}
	}
	return false, false
}

// nodeGenericAttrsNeedRestore stays conservative for the same reason.
func nodeGenericAttrsNeedRestore(node *data.Node, _ string, _ *ExtendedFileInfo) (differs, appliable bool) {
	return len(node.GenericAttributes) > 0, true
}
