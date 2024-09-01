package fs

import (
	"syscall"

	"github.com/restic/restic/internal/restic"
)

func nodeRestoreSymlinkTimestamps(_ string, _ [2]syscall.Timespec) error {
	return nil
}

// nodeRestoreExtendedAttributes is a no-op on openbsd.
func nodeRestoreExtendedAttributes(_ *restic.Node, _ string) error {
	return nil
}

// nodeFillExtendedAttributes is a no-op on openbsd.
func nodeFillExtendedAttributes(_ *restic.Node, _ string, _ bool) error {
	return nil
}

// nodeRestoreGenericAttributes is no-op on openbsd.
func nodeRestoreGenericAttributes(node *restic.Node, _ string, warn func(msg string)) error {
	return restic.HandleAllUnknownGenericAttributesFound(node.GenericAttributes, warn)
}

// fillGenericAttributes is a no-op on openbsd.
func nodeFillGenericAttributes(_ *restic.Node, _ string, _ *ExtendedFileInfo) (allowExtended bool, err error) {
	return true, nil
}
