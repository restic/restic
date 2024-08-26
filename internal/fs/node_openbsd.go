package fs

import (
	"os"
	"syscall"

	"github.com/restic/restic/internal/restic"
)

func nodeRestoreSymlinkTimestamps(_ string, _ [2]syscall.Timespec) error {
	return nil
}

func (s statT) atim() syscall.Timespec { return s.Atim }
func (s statT) mtim() syscall.Timespec { return s.Mtim }
func (s statT) ctim() syscall.Timespec { return s.Ctim }

// nodeRestoreExtendedAttributes is a no-op on openbsd.
func nodeRestoreExtendedAttributes(_ *restic.Node, _ string) error {
	return nil
}

// nodeFillExtendedAttributes is a no-op on openbsd.
func nodeFillExtendedAttributes(_ *restic.Node, _ string, _ bool) error {
	return nil
}

// IsListxattrPermissionError is a no-op on openbsd.
func IsListxattrPermissionError(_ error) bool {
	return false
}

// nodeRestoreGenericAttributes is no-op on openbsd.
func nodeRestoreGenericAttributes(node *restic.Node, _ string, warn func(msg string)) error {
	return restic.HandleAllUnknownGenericAttributesFound(node.GenericAttributes, warn)
}

// fillGenericAttributes is a no-op on openbsd.
func nodeFillGenericAttributes(_ *restic.Node, _ string, _ os.FileInfo, _ *statT) (allowExtended bool, err error) {
	return true, nil
}
