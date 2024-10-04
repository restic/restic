package fs

import "github.com/restic/restic/internal/restic"

// nodeRestoreExtendedAttributes is a no-op on netbsd.
func nodeRestoreExtendedAttributes(_ *restic.Node, _ string) error {
	return nil
}

// nodeFillExtendedAttributes is a no-op on netbsd.
func nodeFillExtendedAttributes(_ *restic.Node, _ string, _ bool) error {
	return nil
}

// nodeRestoreGenericAttributes is no-op on netbsd.
func nodeRestoreGenericAttributes(node *restic.Node, _ string, warn func(msg string)) error {
	return restic.HandleAllUnknownGenericAttributesFound(node.GenericAttributes, warn)
}

// nodeFillGenericAttributes is a no-op on netbsd.
func nodeFillGenericAttributes(_ *restic.Node, _ string, _ *ExtendedFileInfo) (allowExtended bool, err error) {
	return true, nil
}
