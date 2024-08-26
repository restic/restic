package restic

import (
	"os"
	"syscall"
)

func nodeRestoreSymlinkTimestamps(_ string, _ [2]syscall.Timespec) error {
	return nil
}

func (s statT) atim() syscall.Timespec { return s.Atimespec }
func (s statT) mtim() syscall.Timespec { return s.Mtimespec }
func (s statT) ctim() syscall.Timespec { return s.Ctimespec }

// nodeRestoreExtendedAttributes is a no-op on netbsd.
func nodeRestoreExtendedAttributes(_ *Node, _ string) error {
	return nil
}

// nodeFillExtendedAttributes is a no-op on netbsd.
func nodeFillExtendedAttributes(_ *Node, _ string, _ bool) error {
	return nil
}

// IsListxattrPermissionError is a no-op on netbsd.
func IsListxattrPermissionError(_ error) bool {
	return false
}

// nodeRestoreGenericAttributes is no-op on netbsd.
func nodeRestoreGenericAttributes(node *Node, _ string, warn func(msg string)) error {
	return HandleAllUnknownGenericAttributesFound(node.GenericAttributes, warn)
}

// nodeFillGenericAttributes is a no-op on netbsd.
func nodeFillGenericAttributes(_ *Node, _ string, _ os.FileInfo, _ *statT) (allowExtended bool, err error) {
	return true, nil
}
